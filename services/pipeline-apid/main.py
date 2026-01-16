from flask import Flask, request, jsonify
import pymysql
import os
import logging

app = Flask(__name__)

MYSQL_HOST = os.getenv('MYSQL_HOST', 'mysql-pipeline')
MYSQL_PORT = int(os.getenv('MYSQL_PORT', 3306))
MYSQL_USER = os.getenv('MYSQL_USER', 'shop_user')
MYSQL_PASSWORD = os.getenv('MYSQL_PASSWORD', 'shop_password')
MYSQL_DATABASE = os.getenv('MYSQL_DATABASE', 'pipeline_db')

def get_db():
    return pymysql.connect(
        host=MYSQL_HOST,
        port=MYSQL_PORT,
        user=MYSQL_USER,
        password=MYSQL_PASSWORD,
        database=MYSQL_DATABASE,
        cursorclass=pymysql.cursors.DictCursor
    )

@app.route('/api/v1/pipeline/files/register', methods=['POST'])
def register_file():
    data = request.get_json()
    filename = data.get('filename')
    shards = data.get('shards')

    if not filename or not shards:
        return jsonify({'error': 'Missing filename or shards'}), 400

    conn = get_db()
    try:
        with conn.cursor() as cursor:
            for shard in shards:
                sql = """
                INSERT INTO pipeline_tracking (filename, shard, event_ingest_status)
                VALUES (%s, %s, 'started')
                ON DUPLICATE KEY UPDATE filename = %s
                """
                cursor.execute(sql, (filename, shard, filename))
        conn.commit()
        return jsonify({'filename': filename, 'shards': shards, 'created': True}), 200
    except Exception as e:
        conn.rollback()
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/stages/event-ingest/done', methods=['POST'])
def event_ingest_done():
    data = request.get_json()
    filename = data.get('filename')

    if not filename:
        return jsonify({'error': 'Missing filename'}), 400

    conn = get_db()
    try:
        with conn.cursor() as cursor:
            sql = """
            UPDATE pipeline_tracking
            SET event_ingest_status = 'done', updated_at = NOW()
            WHERE filename = %s
            """
            cursor.execute(sql, (filename,))
            updated_rows = cursor.rowcount
        conn.commit()
        return jsonify({'filename': filename, 'updated_rows': updated_rows}), 200
    except Exception as e:
        conn.rollback()
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/stages/<stage>/<transition>', methods=['POST'])
def stage_transition(stage, transition):
    valid_stages = ['shard-splitter', 'shard-worker', 'mysql-sender']
    valid_transitions = ['start', 'done', 'fail']

    if stage not in valid_stages:
        return jsonify({'error': 'Invalid stage'}), 400
    if transition not in valid_transitions:
        return jsonify({'error': 'Invalid transition'}), 400

    data = request.get_json()
    filename = data.get('filename')
    shard = data.get('shard')

    if filename is None or shard is None:
        return jsonify({'error': 'Missing filename or shard'}), 400

    stage_column = {
        'shard-splitter': 'shard_splitter_status',
        'shard-worker': 'shard_worker_status',
        'mysql-sender': 'mysql_sender_status'
    }[stage]

    status_map = {'start': 'started', 'done': 'done', 'fail': 'failed'}
    new_status = status_map[transition]

    conn = get_db()
    try:
        with conn.cursor() as cursor:
            sql = f"""
            UPDATE pipeline_tracking
            SET {stage_column} = %s, updated_at = NOW()
            WHERE filename = %s AND shard = %s
            """
            cursor.execute(sql, (new_status, filename, shard))
            updated_rows = cursor.rowcount
        conn.commit()
        if updated_rows == 0:
            return jsonify({'error': 'No record found'}), 404
        return jsonify({
            'filename': filename,
            'shard': shard,
            'stage': stage,
            'status': new_status
        }), 200
    except Exception as e:
        conn.rollback()
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/stages/<stage>/retry', methods=['POST'])
def stage_retry(stage):
    valid_stages = ['shard-splitter', 'shard-worker', 'mysql-sender']
    if stage not in valid_stages:
        return jsonify({'error': 'Invalid stage'}), 400

    data = request.get_json()
    filename = data.get('filename')
    shard = data.get('shard')

    if filename is None or shard is None:
        return jsonify({'error': 'Missing filename or shard'}), 400

    stage_column = {
        'shard-splitter': 'shard_splitter_status',
        'shard-worker': 'shard_worker_status',
        'mysql-sender': 'mysql_sender_status'
    }[stage]

    conn = get_db()
    try:
        with conn.cursor() as cursor:
            sql = f"""
            UPDATE pipeline_tracking
            SET {stage_column} = 'new', updated_at = NOW()
            WHERE filename = %s AND shard = %s AND {stage_column} = 'failed'
            """
            cursor.execute(sql, (filename, shard))
            updated_rows = cursor.rowcount
        conn.commit()
        if updated_rows == 0:
            return jsonify({'error': 'No failed record found'}), 404
        return jsonify({
            'filename': filename,
            'shard': shard,
            'stage': stage,
            'status': 'new'
        }), 200
    except Exception as e:
        conn.rollback()
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/queues/<stage>', methods=['GET'])
def get_queue(stage):
    valid_stages = ['shard-splitter', 'shard-worker', 'mysql-sender']
    if stage not in valid_stages:
        return jsonify({'error': 'Invalid stage'}), 400

    conditions = {
        'shard-splitter': "event_ingest_status='done' AND shard_splitter_status='new'",
        'shard-worker': "shard_splitter_status='done' AND shard_worker_status='new'",
        'mysql-sender': "shard_worker_status='done' AND mysql_sender_status='new'"
    }

    limit = request.args.get('limit', default=10, type=int)
    offset = request.args.get('offset', default=0, type=int)

    conn = get_db()
    try:
        with conn.cursor() as cursor:
            sql = f"""
            SELECT filename, shard, created_at
            FROM pipeline_tracking
            WHERE {conditions[stage]}
            ORDER BY created_at
            LIMIT %s OFFSET %s
            """
            cursor.execute(sql, (limit, offset))
            items = cursor.fetchall()
        return jsonify({'stage': stage, 'items': items}), 200
    except Exception as e:
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/files/<filename>', methods=['GET'])
def get_file_status(filename):
    conn = get_db()
    try:
        with conn.cursor() as cursor:
            sql = """
            SELECT shard, event_ingest_status, shard_splitter_status,
                   shard_worker_status, mysql_sender_status, updated_at
            FROM pipeline_tracking
            WHERE filename = %s
            ORDER BY shard
            """
            cursor.execute(sql, (filename,))
            shards = cursor.fetchall()
        return jsonify({'filename': filename, 'shards': shards}), 200
    except Exception as e:
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

@app.route('/api/v1/pipeline/metrics/summary', methods=['GET'])
def get_metrics_summary():
    conn = get_db()
    try:
        with conn.cursor() as cursor:
            cursor.execute("SELECT COUNT(*) as total FROM pipeline_tracking")
            total = cursor.fetchone()['total']

            stages = ['shard_splitter_status', 'shard_worker_status', 'mysql_sender_status']
            by_stage = {}
            for stage in stages:
                sql = f"""
                SELECT
                  SUM({stage}='new') as new,
                  SUM({stage}='started') as started,
                  SUM({stage}='done') as done,
                  SUM({stage}='failed') as failed
                FROM pipeline_tracking
                """
                cursor.execute(sql)
                by_stage[stage] = cursor.fetchone()

            sql = """
            SELECT shard,
                   COUNT(*) as total,
                   SUM(mysql_sender_status='done') as done,
                   SUM(mysql_sender_status='failed') as failed
            FROM pipeline_tracking
            GROUP BY shard
            ORDER BY shard
            """
            cursor.execute(sql)
            by_shard = cursor.fetchall()

        return jsonify({
            'total': total,
            'by_stage': by_stage,
            'by_shard': by_shard
        }), 200
    except Exception as e:
        return jsonify({'error': str(e)}), 500
    finally:
        conn.close()

if __name__ == '__main__':
    app.run(host='0.0.0.0', port=8083)