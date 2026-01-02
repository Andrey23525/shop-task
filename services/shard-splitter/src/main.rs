use axum::{
    extract::{Extension, Json},
    http::StatusCode,
    response::IntoResponse,
    routing::post,
    Router,
};
use serde::{Deserialize, Serialize};
use sqlx::mysql::MySqlPool;
use std::net::SocketAddr;

#[derive(Debug, Deserialize)]
struct IngestRequest {
    filename: String,
    shards: Vec<i32>,
}

#[derive(Debug, Serialize)]
struct IngestResponse {
    filename: String,
    shards: Vec<i32>,
    created: bool,
}

async fn files_register(
    Extension(pool): Extension<MySqlPool>,
    Json(payload): Json<IngestRequest>,
) -> Result<impl IntoResponse, StatusCode> {
    println!("{:?}", payload);
    let mut any_created = false;

    for shard in &payload.shards {
        let result = sqlx::query(
            r#"
            INSERT INTO pipeline_tracking (
                filename,
                shard,
                event_ingest_status,
                transform_status,
                load_status
            )
            VALUES (?, ?, 'started', 'new', 'new')
            ON DUPLICATE KEY UPDATE filename = filename
            "#,
        )
            .bind(&payload.filename)
            .bind(shard)
            .execute(&pool)
            .await
            .map_err(|e| {
                eprintln!("Database error: {:?}", e);
                StatusCode::INTERNAL_SERVER_ERROR
            })?;

        // rows_affected() вернёт 1 если запись вставлена, 2 если была обновлена
        if result.rows_affected() == 1 {
            any_created = true;
        }
    }

    let response = IngestResponse {
        filename: payload.filename,
        shards: payload.shards,
        created: any_created,
    };

    Ok((StatusCode::OK, Json(response)))
}

#[derive(Debug, Deserialize)]
struct IngestDoneRequest {
    filename: String
}

#[derive(Debug, Serialize)]
struct IngestDoneResponse {
    filename: String,
    updated_rows: u64
}

async fn stages_event_ingest_done(
    Extension(pool): Extension<MySqlPool>,
    Json(payload): Json<IngestDoneRequest>,
) -> Result<impl IntoResponse, StatusCode> {
    println!("{:?}", payload);

    let result = sqlx::query(
        r#"
        UPDATE pipeline_tracking
        SET event_ingest_status = 'done', updated_at = NOW()
        WHERE filename = ?;
        "#,
    )
        .bind(&payload.filename)
        .execute(&pool)
        .await
        .map_err(|e| {
            eprintln!("Database error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        })?;

    let count = result.rows_affected();

    let response = IngestDoneResponse {
        filename: payload.filename,
        updated_rows: count
    };

    Ok((StatusCode::OK, Json(response)))
}

#[tokio::main]
async fn main() {
    let database_url = std::env::var("DATABASE_URL")
        .unwrap_or_else(|_| "mysql://shop_user:shop_password@mysql-pipeline:3306/pipeline_db".to_string());

    let pool = sqlx::mysql::MySqlPoolOptions::new()
        .max_connections(5)
        .connect(&database_url)
        .await
        .expect("Не удалось подключиться к БД");

    sqlx::query(
        r#"
            CREATE TABLE IF NOT EXISTS pipeline_tracking (
    id INT AUTO_INCREMENT PRIMARY KEY,
    filename VARCHAR(255) NOT NULL,
    shard INT NOT NULL,
    event_ingest_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
    transform_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
    load_status ENUM('new', 'started', 'done', 'failed') DEFAULT 'new',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY unique_filename_shard (filename, shard)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
            "#,
    )
        .execute(&pool)
        .await
        .map_err(|e| {
            eprintln!("Database error: {:?}", e);
            StatusCode::INTERNAL_SERVER_ERROR
        }).unwrap();

    let app: Router = Router::new()
        // .route("/files/register", post(files_register))
        // .route("/stages/event-ingest/done", post(stages_event_ingest_done))
        .layer(Extension(pool));

    let addr = SocketAddr::from(([0, 0, 0, 0], 8082));
    println!("Сервер запущен на http://{}", addr);

    let listener = tokio::net::TcpListener::bind(addr)
        .await
        .unwrap();

    loop {}
    // axum::serve(listener, app)
    //     .await
    //     .unwrap();
}

// /api/v1/pipeline/files/register