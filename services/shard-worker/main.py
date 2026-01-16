import os
import time
import json
import logging
import requests
from pathlib import Path
from collections import defaultdict

PIPELINE_API_URL = os.getenv('PIPELINE_API_URL', 'http://pipeline-apid:8082/api/v1/pipeline')
SHARED_DIR = os.getenv('SHARED_DIR', '/shared')
SHARD_ID = int(os.getenv('SHARD_ID', 0))
SPLITTER_DIR = os.path.join(SHARED_DIR, 'shard-splitter')
WORKER_DIR = os.path.join(SHARED_DIR, f'shard-worker-{SHARD_ID}')

Path(WORKER_DIR).mkdir(parents=True, exist_ok=True)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

cache = defaultdict(set)
CACHE_TTL = 30

def process_task(filename, shard):
    response = requests.post(
        f"{PIPELINE_API_URL}/stages/shard-worker/start",
        json={"filename": filename, "shard": shard}
    )
    if response.status_code != 200:
        logger.error(f"Failed to start shard-worker for {filename} shard {shard}: {response.text}")
        return

    input_file = os.path.join(SPLITTER_DIR, f"{filename}.shard-splitter.{shard}.txt")
    output_file = os.path.join(WORKER_DIR, f"{filename}.shard-worker.{shard}.prepared.txt")

    try:
        with open(input_file, 'r') as f:
            events = f.readlines()

        processed_events = []
        for line in events:
            event = json.loads(line)
            event_type = event.get('event_type')
            payload = event.get('payload', {})
            timestamp = event.get('timestamp')

            if event_type in [1, 3]:
                order_id = payload.get('order_id')
                if order_id is not None:
                    key = f"{order_id}_{timestamp}"
                    if key in cache[event_type]:
                        continue
                    cache[event_type].add(key)

            processed_event = {
                'event_type': event_type,
                'shop_id': payload.get('shop_id'),
                'user_id': payload.get('user_id'),
                'timestamp': timestamp,
                'shard': shard
            }
            processed_events.append(json.dumps(processed_event) + '\n')

        with open(output_file, 'w') as f:
            f.writelines(processed_events)

        response = requests.post(
            f"{PIPELINE_API_URL}/stages/shard-worker/done",
            json={"filename": filename, "shard": shard}
        )
        if response.status_code != 200:
            logger.error(f"Failed to mark shard-worker done for {filename} shard {shard}: {response.text}")
        else:
            logger.info(f"Processed {filename} for shard {shard}")

    except Exception as e:
        logger.error(f"Error processing {filename} shard {shard}: {e}")
        response = requests.post(
            f"{PIPELINE_API_URL}/stages/shard-worker/fail",
            json={"filename": filename, "shard": shard}
        )

def main():
    logger.info(f"Starting shard-worker daemon for shard {SHARD_ID}")
    while True:
        try:
            response = requests.get(f"{PIPELINE_API_URL}/queues/shard-worker")
            if response.status_code == 200:
                tasks = response.json().get('items', [])
                for task in tasks:
                    if task['shard'] == SHARD_ID:
                        process_task(task['filename'], task['shard'])
            else:
                logger.error(f"Failed to get queue: {response.text}")
        except Exception as e:
            logger.error(f"Error in main loop: {e}")
        time.sleep(5)

if __name__ == '__main__':
    main()