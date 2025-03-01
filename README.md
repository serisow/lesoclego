docker compose down && cd app/ && GOOS=linux go build -o lesoclego && cd .. && docker compose up



EXAMPLE call one-time execution:

curl -X POST http://lesoclego-dev.sa/pipeline/test_first_on_demand/execute \
     -H "Content-Type: application/json" \
     -d '{"user_input": "Analyze the impact of artificial intelligence on job markets"}'


     {"execution_id":"aa90167b-4f2a-4915-8e30-f50e094ab11c","links":{"results":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results","self":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c","status":"/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status"},"pipeline_id":"test_first_on_demand","status":"started","submitted_at":"2024-10-18T19:58:03Z","user_input":"Write a story about Agentic Workflow"}


http://localhost:8086/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/results
http://localhost:8086/pipeline/test_first_on_demand/execution/aa90167b-4f2a-4915-8e30-f50e094ab11c/status




# Backup from inside the container
docker exec lesoclego_postgres pg_dump -U $DB_USER -d $DB_NAME -F c -b -v > ./backups/backup_$(date +%Y%m%d_%H%M%S).dump
