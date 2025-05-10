set -ex

CUBE_WORKER_HOST=localhost
CUBE_WORKER_PORT=5556
CUBE_MANAGER_HOST=localhost
CUBE_MANAGER_PORT=5555

SCREEN_SESSION=orchestrator_tests

# Start a worker
screen -dm -S $SCREEN_SESSION ../main worker --host $CUBE_WORKER_HOST --port $CUBE_WORKER_PORT

# Start the manager
screen -S $SCREEN_SESSION -X screen ../main manager --host $CUBE_MANAGER_HOST --port $CUBE_MANAGER_PORT --workers "$CUBE_WORKER_HOST:$CUBE_WORKER_PORT"

# Keep screen windows from closing after tests run
screen -S $SCREEN_SESSION -X zombie qr

# Run worker tests
screen -S $SCREEN_SESSION -X screen bash worker_tests.sh

# Run manager tests
screen -S $SCREEN_SESSION -X screen bash manager_tests.sh
