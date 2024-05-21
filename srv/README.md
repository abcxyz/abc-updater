# Metrics Server

This server has a few parts:

1. Metrics endpoint to accept metric calls.
2. Fetcher to collect information about allowed metrics.
3. Logger which logs metrics into cloud logging.


## Allowed Metrics
Defined in `metrics.json` file hosted next to version info for updater.

`manifest.json` file at root directory of version folder includes a list of
all applications which accept metrics, and is used by server to know which paths
to load.

Metric definitions are loaded on startup and updated periodically.

Example:
```
{
	app_id: abc
	allowed_metrics: [
		"metric_name_1",
		"metric_name_2",
	]
}
```

