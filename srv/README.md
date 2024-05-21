# Metrics Server

This server has a few parts:

1. Metrics endpoint to accept metric calls.
2. Fetcher to collect information about allowed metrics.
3. Logger which logs metrics into cloud logging.


## Allowed Metrics
Defined in `metrics.json` file hosted next to version info for updater.

Example:
```
# TODO: should types be defined/enforced?
{
	app_id: abc
	allowed_metrics: [
		"metric_name_1",
		"metric_name_2",
	]
}
```

Server will look up the relevant `metrics.json` file when fielding a request.
The results of this lookup will be cached (including 404).

TBD: How to avoid DOS from lookups of arbitrary application names. This could
cause expensive lookups, as well as overfill the cache.

TBD: Is there an easy way to get a mainifest of all apps from the update server?

