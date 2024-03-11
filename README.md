# ABC Updater

**This is not an official Google product.**

## Description
ABC updater is an embeddable library to notify application users if there is
a newer version available.

In the future, it may include security bulletin features, and coarse metrics.

It is intended to be run with a separate backend, where application information
is stored in JSON. Version checks and notifications will only happen once per
day.

### Opt Out
You can opt out of update notifications. Every application consuming
`abc updater` will define an `app id`. `toUpperCase(id)` can be used in an
env var to do so.

For example, if an app had id `foo_bar_123`, you could opt out by setting the
env var `FOO_BAR_123_IGNORE_VERSIONS`.

This is most easily shown by example:

```shell
# If newest version is 1.0.7, don't notify
FOO_BAR_123_IGNORE_VERSIONS=1.0.7
# Don't notify unless new version if at least 2.0.0
FOO_BAR_123_IGNORE_VERSIONS=<2.0.0
# Don't check for new versions at all.
FOO_BAR_123_IGNORE_VERSIONS=ALL

# You can combine multiple constraints with commas:
# Don't notify if new version is 1.9.3 or 2.0.4 
FOO_BAR_123_IGNORE_VERSIONS=1.9.3,2.0.4
```

### Limitations
Currently, only the newest version is fetched. This means that if you are on
`1.3.0` and some fix `1.4.0` was released after a `2.0` was released, you would
only be notified of the `2.0`, and if you ignored `2.0`, you still would not 
see the `1.4.0`.
