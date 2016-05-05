#  pg_rewind

Stolon can use [pg_rewind](http://www.postgresql.org/docs/current/static/app-pgrewind.html) to speedup instance resynchronization (for example resyncing an old master or a slave ahead of the current master) without the need to copy all the new master data.

## Enabling

It can be enabled setting to true the cluster config option `use_pg_rewind` (disabled by default). So you should enable it in the cluster config:

``` bash
stolonctl --cluster-name=mycluster config patch '{ "use_pg_rewind" : true }'
```

This will also enable the `wal_log_hints` postgresql parameter. If previously `wal_log_hints` wasn't enabled you should restart the postgresql instances (you can do so restarting the `stolon-keeper`)

pg_rewind needs to connect to the master database with a superuser role.
Actually only password authentication is supported. In future different authentication mechanism will be added.

* It'll use the superuser role provided with `--pg-su-username` (if not specified it'll default to the os user name running the stolon-keeper). This superuser role (if not existing) needs to be created on the master database.
* The superuser credentials need to be provided to the `stolon-keeper`.

Actually to avoid security problems (superuser credential cannot be globally defined in the cluster config since actually it can be read by anyone accessing the cluster store) you have to set the superuser name and password when executing the `stolon-keeper`:
* Providing the `--pg-su-username` and `--pg-su-passwordfile` (preferred) or `--pg-su-password` options (discouraged since every user accessing the system can read the password with a simple `ps`)
* Exporting the `STKEEPER_PG_SU_USERNAME` and `STKEEPER_PG_SU_PASSWORD` environment variables.




