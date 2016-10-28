## Simple Cluster

These are the steps to setup and test a simple cluster.

This example assumes a running etcd or consul server on localhost

Note: under ubuntu the `initdb` command is not provided in the path. You should updated the exported `PATH` env variable or provide the `--pg-bin-path` command line option to the `stolon-keeper` command.

### Initialize the cluster

The first step is to initialize a cluster with a cluster specification. For now we'll just initialize a cluster without providing a cluster specification but using a default one that will just start with an empty database cluster.

```
./bin/stolonctl --cluster-name stolon-cluster --store-backend=etcd init
```

If you want to automate this step you can just pass an initial cluster specification to the sentinels with the `--initial-cluster-spec` option.

### Start a sentinel

The sentinel will become the sentinels leader for the cluster named `stolon-cluster`.
```
./bin/stolon-sentinel --cluster-name stolon-cluster --store-backend=etcd
```

```
sentinel: id: 336d6e14
sentinel: trying to acquire sentinels leadership
sentinel: sentinel leadership acquired
```

### Launch first keeper

```
./bin/stolon-keeper --cluster-name stolon-cluster --store-backend=etcd --id postgres0 --data-dir data/postgres0 --pg-su-password=supassword --pg-repl-username=repluser --pg-repl-password=replpassword
```

This will start a stolon keeper with id `postgres0` listening by default on localhost:5431, it will setup and initialize a postgres instance inside `data/postgres0/postgres/`

```
keeper: id: postgres0
postgresql: Stopping database
keeper: our keeper data is not available, waiting for it to appear
keeper: current db UID "" different than cluster data db UID "30376139", initializing the database cluster
postgresql: Setting required accesses to pg_hba.conf
postgresql: Starting database
postgresql: Setting roles
postgresql: Defining superuser password
postgresql: Creating replication role
postgresql: Stopping database
keeper: our db requested role is master
postgresql: Starting database
```

The sentinel will elect it as the master instance:

```
sentinel: trying to find initial master
sentinel: initializing cluster using keeper "postgres0" as master db owner
```

### Start a proxy

Now we can start the proxy

```
./bin/stolon-proxy --cluster-name stolon-cluster --store-backend=etcd --port 25432
```

```
proxy: id: a8f586a9
proxy: Starting proxying
proxy: master address: 127.0.0.1:5432
```


### Connect to the proxy service

Since this simple cluster is running on localhost, the current user (which started the first keeper) is the same superuser created at db initialization and the default pg_hba.conf trusts it, you can login without a password.

```
psql --host localhost --port 25432 postgres
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
./bin/stolon-keeper --cluster-name stolon-cluster --store-backend=etcd --id postgres1 --data-dir data/postgres1 --pg-su-password=supassword --pg-repl-username=repluser --pg-repl-password=replpassword --port 5433 --pg-port 5435
```

This instance will start replicating from the master (postgres0)

### Simulate master death

You can now try killing the actual keeper managing the master postgres instance (if killed with SIGKILL it'll leave an unmanaged postgres instance, if you terminate it with SIGTERM it'll also stop the postgres instance before exiting) (if you just kill postgres, it'll be restarted by the keeper usually before the sentinels declares it as not healty) noticing that the sentinels will:

* declare the master as not healty.
* elect the standby as the new master.
* Remove the proxyconf. The current `psql` connection will be dropped.
* Wait for the new master to be ready.
* Update the proxyconf with the new master address.


```
sentinel: master db "30376139" on keeper "postgres0" is failed
sentinel: trying to find a standby to replace failed master
sentinel: electing db "64646634" on keeper "postgres1" as the new master
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
