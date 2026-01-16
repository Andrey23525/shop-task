import os
import json
import time
import logging
import signal
import shutil
import schedule
import requests
import mysql.connector
from pathlib import Path
from dotenv import load_dotenv

load_dotenv()

PIPELINE_API_URL = os.getenv("PIPELINE_API_URL", "http://pipeline-apid:8082/api/v1/pipeline")
MYSQL_SHARD_0_HOST = os.getenv("MYSQL_SHARD_0_HOST", "mysql-shard-0")
MYSQL_SHARD_1_HOST = os.getenv("MYSQL_SHARD_1_HOST", "mysql-shard-1")
MYSQL_PORT = int(os.getenv("MYSQL_PORT", "3306"))
MYSQL_USER = os.getenv("MYSQL_USER", "shop_user")
MYSQL_PASSWORD = os.getenv("MYSQL_PASSWORD", "shop_password")
MYSQL_DATABASE_PREFIX = os.getenv("MYSQL_DATABASE_PREFIX", "shop_shard_")
POLL_INTERVAL = int(os.getenv("POLL_INTERVAL", "5"))
SHARED_DIR = Path("/shared")
SERVICE_DIR = SHARED_DIR / "mysql-sender"

SERVICE_DIR.mkdir(parents=True, exist_ok=True)

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler(SERVICE_DIR / "activity.log"),
        logging.StreamHandler()
    ]
)
logger = logging.getLogger(__name__)

def cleanup():
    if SERVICE_DIR.exists():
        shutil.rmtree(SERVICE_DIR)
        logger.info(f"Cleaned up {SERVICE_DIR}")

def signal_handler(signum, frame):
    cleanup()
    exit(0)

signal.signal(signal.SIGTERM, signal_handler)
signal.signal(signal.SIGINT, signal_handler)

mysql_connections = {
    0: mysql.connector.connect(
        host=MYSQL_SHARD_0_HOST,
        port=MYSQL_PORT,
        user=MYSQL_USER,
        password=MYSQL_PASSWORD,
        database=f"{MYSQL_DATABASE_PREFIX}0"
    ),
    1: mysql.connector.connect(
        host=MYSQL_SHARD_1_HOST,
        port=MYSQL_PORT,
        user=MYSQL_USER,
        password=MYSQL_PASSWORD,
        database=f"{MYSQL_DATABASE_PREFIX}1"
    )
}

def get_queue():
    try:
        response = requests.get(f"{PIPELINE_API_URL}/queues/mysql-sender", timeout=10)
        response.raise_for_status()
        return response.json()["items"]
    except Exception as e:
        logger.error(f"Failed to fetch queue: {e}")
        return []

def update_status(filename, shard, transition):
    try:
        response = requests.post(
            f"{PIPELINE_API_URL}/stages/mysql-sender/{transition}",
            json={"filename": filename, "shard": shard},
            timeout=5
        )
        response.raise_for_status()
        logger.info(f"Updated {filename} shard {shard} to {transition}")
    except Exception as e:
        logger.error(f"Failed to update status: {e}")

def insert_events(shard, events):
    conn = mysql_connections[shard]
    cursor = conn.cursor()
    try:
        insert_query = """
            INSERT INTO events (event_type, shop_id, user_id, timestamp, shard, created_at)
            VALUES (%s, %s, %s, %s, %s, NOW())
        """
        data = []
        for event in events:
            data.append((
                event["event_type"],
                event.get("shop_id"),
                event.get("user_id"),
                event["timestamp"],
                shard
            ))
        cursor.executemany(insert_query, data)
        conn.commit()
        logger.info(f"Inserted {len(events)} events into shard {shard}")
    except Exception as e:
        conn.rollback()
        raise e
    finally:
        cursor.close()

def process_task(filename, shard):
    input_file = SHARED_DIR / f"shard-worker-{shard}" / f"{filename}.shard-worker.{shard}.prepared.txt"
    if not input_file.exists():
        raise FileNotFoundError(f"Input file not found: {input_file}")

    events = []
    with open(input_file, 'r') as f:
        for line in f:
            line = line.strip()
            if line:
                events.append(json.loads(line))

    if events:
        insert_events(shard, events)

def process_queue():
    tasks = get_queue()
    for task in tasks:
        filename = task["filename"]
        shard = task["shard"]
        try:
            update_status(filename, shard, "start")
            process_task(filename, shard)
            update_status(filename, shard, "done")
        except Exception as e:
            logger.error(f"Failed to process {filename} shard {shard}: {e}")
            update_status(filename, shard, "fail")

def main():
    logger.info("MySQL Sender started")
    schedule.every(POLL_INTERVAL).seconds.do(process_queue)
    while True:
        schedule.run_pending()
        time.sleep(1)

if __name__ == "__main__":
    main()