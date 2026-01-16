import os
import time
import json
import logging
import requests
from pathlib import Path

PIPELINE_API_URL = os.getenv('PIPELINE_API_URL', 'http://pipeline-apid:8082/api/v1/pipeline')
SHARED_DIR = os.getenv('SHARED_DIR', '/shared')
EVENTS_DIR = os.path.join(SHARED_DIR, 'events')
SPLITTER_DIR = os.path.join(SHARED_DIR, 'shard-splitter')

Path(SPLITTER_DIR).mkdir(parents=True, exist_ok=True)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

def process_task(filename, shard):
    response = requests.post(
        f"{PIPELINE_API_URL}/stages/shard-splitter/start",
        json={"filename": filename, "shard": shard}
    )
    if response.status_code != 200:
        logger.error(f"Failed to start shard-splitter for {filename} shard {shard}: {response.text}")
        return

    input_file = os.path.join(EVENTS_DIR, f"{filename}.event-ingest.txt")
    output_file = os.path.join(SPLITTER_DIR, f"{filename}.shard-splitter.{shard}.txt")

    try:
        with open(input_file, 'r') as f:
            events = f.readlines()

        filtered_events = []
        for line in events:
            event = json.loads(line)
            shop_id = event.get('payload', {}).get('shop_id')
            if shop_id is not None and shop_id % 2 == shard:
                filtered_events.append(line)

        with open(output_file, 'w') as f:
            f.writelines(filtered_events)

        response = requests.post(
            f"{PIPELINE_API_URL}/stages/shard-splitter/done",
            json={"filename": filename, "shard": shard}
        )
        if response.status_code != 200:
            logger.error(f"Failed to mark shard-splitter done for {filename} shard {shard}: {response.text}")
        else:
            logger.info(f"Processed {filename} for shard {shard}")

    except Exception as e:
        logger.error(f"Error processing {filename} shard {shard}: {e}")
        response = requests.post(
            f"{PIPELINE_API_URL}/stages/shard-splitter/fail",
            json={"filename": filename, "shard": shard}
        )

def main():
    logger.info("Starting shard-splitter daemon")
    while True:
        try:
            response = requests.get(f"{PIPELINE_API_URL}/queues/shard-splitter")
            if response.status_code == 200:
                tasks = response.json().get('items', [])
                for task in tasks:
                    process_task(task['filename'], task['shard'])
            else:
                logger.error(f"Failed to get queue: {response.text}")
        except Exception as e:
            logger.error(f"Error in main loop: {e}")
        time.sleep(5)

if __name__ == '__main__':
    main()