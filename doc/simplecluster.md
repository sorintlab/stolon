## Simple Cluster

These are the steps to setup and test a simple cluster.

This example assumes a running etcd server on localhost

Note: under ubuntu the `initdb` command is not provided in the path. You should updated the exported `PATH` env variable or provide the `--pg-bin-path` command line option to the `stolon-keeper` command.

### Start a sentinel

The sentinel will become the sentinels leader for `stolon-cluster` and initialize the first clusterview.

```
./bin/stolon-sentinel --cluster-name stolon-cluster
```

```
sentinel: id: 336d6e14
sentinel: trying to acquire sentinels leadership
sentinel: sentinel leadership acquired
```

### Launch first keeper

```
./bin/stolon-keeper --data-dir data/postgres0 --id postgres0 --cluster-name stolon-cluster
```

This will start a stolon keeper with id `postgres0` listening by default on localhost:5431, it will setup and initialize a postgres instance inside `data/postgres0/postgres/`

```
keeper: generated id: postgres0
keeper: id: postgres0
postgresql: Stopping database
keeper: Initializing database
postgresql: Setting required accesses to pg_hba.conf
postgresql: Starting database
postgresql: Creating repl user
postgresql: Stopping database
postgresql: Starting database
```

The sentinel will elect it as the master instance:

```
sentinel: Initializing cluster with master: "postgres0"
sentinel: Updating proxy view to localhost:5432
sentinel: I'm the sentinels leader
```

### Start a proxy

Now we can start the proxy

```
./bin/stolon-proxy --cluster-name stolon-cluster --port 25432
```

```
pgproxy: id: 4b321666
pgproxy: addr: 127.0.0.1:5432
```


### Connect to the proxy service

Since this simple cluster is running on localhost, the current user (which started the first keeper) is the same superuser created at db initialization and the default pg_hba.conf trusts it, you can login without a password.

```
psql --host 10.247.50.217 --port 5432 postgres
Password for user stolon:
psql (9.4.5, server 9.4.4)
Type "help" for help.

postgres=#
```

### Create a test table and insert a row

Connect to the db. Create a test table and do some inserts (we use the "postgres" database for these tests but usually this shouldn't be done).

```
postgres=# create table test (id int primary key not null, value text not null);
CREATE TABLE
postgres=# insert into test values (1, 'value1');
INSERT 0 1
postgres=# select * from test;
 id | value
----+--------
  1 | value1
(1 row)
```

### Start another keeper:

```
./bin/stolon-keeper --data-dir data/postgres1 --id postgres1 --cluster-name stolon-cluster --port 5433 --pg-port 5435
```

This instance will start replicating from the master (postgres0)

### Simulate master death

You can now try killing the actual keeper managing the master postgres instance (if killed with SIGKILL it'll leave an unmanaged postgres instance, if you terminate it with SIGTERM it'll also stop the postgres instance before exiting) (if you just kill postgres, it'll be restarted by the keeper usually before the sentinels declares it as not healty) noticing that the sentinels will:

* declare the master as not healty.
* elect the standby as the new master.
* Remove the proxyview. The current `psql` connection will be dropped.
* Wait for the new master to be ready.
* Update the proxyView with the new master address.


```
sentinel: master is failed
sentinel: trying to find a standby to replace failed master
sentinel: electing new master: "postgres1"
sentinel: deleting proxy view
sentinel: updating proxy view to localhost:5435
```

Now, inside the previous `psql` session you can redo the last select. The first time `psql` will report that the connection was closed and then it successfully reconnected:

```
postgres=# select * from test;
server closed the connection unexpectedly
        This probably means the server terminated abnormally
        before or while processing the request.
The connection to the server was lost. Attempting reset: Succeeded.
postgres=# select * from test;
 id | value
----+--------
  1 | value1
(1 row)
```

If you start the `postgres0` keeper it'll read the new cluster view and make the old master become a standby of the new master.
