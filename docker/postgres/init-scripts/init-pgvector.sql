-- Create the user if not exists
DO
$do$
BEGIN
   IF NOT EXISTS (
      SELECT FROM pg_catalog.pg_roles
      WHERE  rolname = 'lesoclego_user') THEN

      CREATE USER lesoclego_user WITH PASSWORD 'lesoclego_pwd';
   END IF;
END
$do$;

-- Create the database if not exists
SELECT 'CREATE DATABASE lesoclego_db'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'lesoclego_db')\gexec

-- Connect to the database
\c lesoclego_db

-- Grant privileges to the user
GRANT ALL PRIVILEGES ON DATABASE lesoclego_db TO lesoclego_user;

-- Create the pgvector extension
CREATE EXTENSION IF NOT EXISTS vector;

-- Create the documents table
CREATE TABLE IF NOT EXISTS documents (
    id SERIAL PRIMARY KEY,
    filename TEXT NOT NULL,
    content TEXT NOT NULL,
    embedding vector(1536)
);

-- Grant privileges on the documents table to the user
GRANT ALL PRIVILEGES ON TABLE documents TO lesoclego_user;
GRANT USAGE, SELECT ON SEQUENCE documents_id_seq TO lesoclego_user;