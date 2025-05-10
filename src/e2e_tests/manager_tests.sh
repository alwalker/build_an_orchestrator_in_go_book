set -ex

CONTAINER_NAME="orchestrator-e2e-test-manager-start-task"
CUBE_WORKER_HOST=localhost
CUBE_WORKER_PORT=5556
CUBE_MANAGER_HOST=localhost
CUBE_MANAGER_PORT=5555

RESULT=$(curl -v -w "%{json}" --request POST \
    --header 'Content-Type: application/json' \
    --data @manager_start_task.json \
    "http://$CUBE_WORKER_HOST:$CUBE_MANAGER_PORT/tasks")
HTTP_CODE=$(echo $RESULT | jq --slurp -r '.[1].http_code')
if [[ "$HTTP_CODE" -eq "201" ]]; then
    echo "Start return code was 201"
else
    echo "Start return code was not 201: $HTTP_CODE"
    exit 1
fi

echo "Waiting for container to start..."
sleep 35

CONTAINER_ID=$(echo $RESULT | jq --slurp -r '.[0].ID')
if [[ "$CONTAINER_ID" == "21b23589-5d2d-1111-b5c9-a97e9832d021" ]]; then
    echo "Container ID's match"
else
    echo "Container ID's do not match"
    exit 1
fi

NUM_MATCHING_CONTAINERS=$(podman container list \
    --filter name=$CONTAINER_NAME \
    --format json | jq -r 'length')
if [[ "$NUM_MATCHING_CONTAINERS" -eq "1" ]]; then
    echo "Exactly one container found running"
else
    echo "Incorrect number of containers found running: $NUM_MATCHING_CONTAINERS"
    exit 1
fi

RESULT=$(curl -v -w "%{json}" "http://$CUBE_WORKER_HOST:$CUBE_MANAGER_PORT/tasks")
CONTAINER_ID=$(echo $RESULT | jq --slurp -r '.[0].[].ID')
if [[ "$CONTAINER_ID" == "21b23589-5d2d-1111-b5c9-a97e9832d021" ]]; then
    echo "Container ID from GET /tasks match"
else
    echo "Container ID Container ID from GET /tasks does not match"
    exit 1
fi

# This is an untested test because delete currently does not work

# RESULT=$(curl -v -w "%{json}" -X DELETE \
#     --url http://$CUBE_WORKER_HOST:$CUBE_MANAGER_PORT/tasks/21b23589-5d2d-1111-b5c9-a97e9832d021)
# HTTP_CODE=$(echo $RESULT | jq --slurp -r '.[0].http_code')
# if [[ "$HTTP_CODE" -eq "204" ]]; then
#     echo "Stop return code was 204"
# else
#     echo "Stop return code was not 201: $HTTP_CODE"
#     exit 1
# fi

RESULT=$(curl -v -w "%{json}" "http://$CUBE_WORKER_HOST:$CUBE_MANAGER_PORT/nodes")
HTTP_CODE=$(echo $RESULT | jq --slurp -r '.[1].http_code')
if [[ "$HTTP_CODE" -eq "200" ]]; then
    echo "Nodes return code was 200"
else
    echo "Nodes return code was not 200: $HTTP_CODE"
    exit 1
fi
WORKER_NODE_NAME=$(echo $RESULT | jq --slurp -r '.[0].[0].Name')
if [[ "$WORKER_NODE_NAME" == "$CUBE_WORKER_HOST:$CUBE_WORKER_PORT" ]]; then
    echo "Valid node returned"
else
    echo "Invalid worker name returned: $WORKER_NODE_NAME"
    exit 1
fi

podman container stop --filter name=$CONTAINER_NAME
podman container rm --filter name=$CONTAINER_NAME
