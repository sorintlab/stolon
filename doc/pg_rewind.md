#  pg_rewind

Stolon can use [pg_rewind](http://www.postgresql.org/docs/current/static/app-pgrewind.html) to speedup instance resynchronization (for example resyncing an old master or a slave ahead of the current master) without the need to copy all the new master data.

## Enabling

It can be enabled setting to true the cluster specification option `usePgrewind` (defaults to false):

``` bash
stolonctl [cluster options] update --patch '{ "usePgrewind" : true }'
```

This will also enable the `wal_log_hints` postgresql parameter. If previously `wal_log_hints` wasn't enabled you should restart the postgresql instances (you can do so restarting the `stolon-keeper`)

pg_rewind needs to connect to the master database with a superuser role (see the [Stolon Architecture and Requirements](architecture.md)).
