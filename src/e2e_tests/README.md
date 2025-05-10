## "End to End" Tests

These are bad.

Currently screen is broken so I couldn't finish testing the full automation.  I would suggest manually starting screen, starting a worker in one window and the manager in another, then run tests in others.

Remember everything that happens in both the worker(s) and managers is on generous timers.

The crux of a lot of issues is that deletes (stopping tasks) aren't being processed properly. So while containers are manually cleaned up via the podman cli this means the manager/workers have to be restarted to clear state.