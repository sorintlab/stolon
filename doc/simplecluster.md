## Simple Cluster

These are the steps to setup and test a simple cluster.

This example assumes a running etcd 3.x server on localhost. We'll use the etcd v3 api to store stolon data inside etcd.

Note: under ubuntu the `initdb` command is not provided in the path. You should updated the exported `PATH` env variable or provide the `--pg-bin-path` command line option to the `stolon-keeper` command.

### Initialize the cluster

The first step is to initialize a cluster with a cluster specification. For now we'll just initialize a cluster without providing a cluster specification but using a default one that will just start with an empty database cluster.

```
./bin/stolonctl --cluster-name stolon-cluster --store-backend=etcdv3 init
```

If you want to automate this step you can just pass an initial cluster specification to the sentinels with the `--initial-cluster-spec` option.

### Start a sentinel

The sentinel will become the sentinels leader for the cluster named `stolon-cluster`.
```
./bin/stolon-sentinel --cluster-name stolon-cluster --store-backend=etcdv3
```

```
sentinel id id=66613766
Trying to acquire sentinels leadership
sentinel leadership acquired
```

### Launch first keeper

```
./bin/stolon-keeper --cluster-name stolon-cluster --store-backend=etcdv3 --uid postgres0 --data-dir data/postgres0 --pg-su-password=supassword --pg-repl-username=repluser --pg-repl-password=replpassword --pg-listen-address=127.0.0.1
```

This will start a stolon keeper with id `postgres0` listening by default on localhost:5431, it will setup and initialize a postgres instance inside `data/postgres0/postgres/`

```
exclusive lock on data dir taken
keeper uid uid=postgres0
stopping database
our keeper data is not available, waiting for it to appear
our keeper data is not available, waiting for it to appear
current db UID different than cluster data db UID db= cdDB=2a87ea79
initializing the database cluster
cannot get configured pg parameters error=dial tcp 127.0.0.1:5432: getsockopt: connection refused
starting database
error getting pg state error=pq: password authentication failed for user "repluser"
setting roles
setting superuser password
superuser password set
creating replication role
replication role created role=repluser
stopping database
our db requested role is master
starting database
already master
postgres parameters changed, reloading postgres instance
reloading database configuration
```

The sentinel will elect it as the master instance:

```
trying to find initial master
initializing cluster keeper=postgres0
received db state for unexpected db uid receivedDB= db=2a87ea79
waiting for db db=2a87ea79 keeper=postgres0
waiting for db db=2a87ea79 keeper=postgres0
waiting for db db=2a87ea79 keeper=postgres0
db initialized db=2a87ea79 keeper=postgres0
```

### Start a proxy

Now we can start the proxy

```
./bin/stolon-proxy --cluster-name stolon-cluster --store-backend=etcdv3 --port 25432
```

```
proxy id id=63323266
Starting proxying
master address address=127.0.0.1:5432
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
./bin/stolon-keeper --cluster-name stolon-cluster --store-backend=etcdv3 --uid postgres1 --data-dir data/postgres1 --pg-su-password=supassword --pg-repl-username=repluser --pg-repl-password=replpassword --pg-listen-address=127.0.0.1 --pg-port 5435
```

This instance will start replicating from the master (postgres0)

```
exclusive lock on data dir taken
keeper uid uid=postgres1
stopping database
our keeper data is not available, waiting for it to appear
our keeper data is not available, waiting for it to appear
current db UID different than cluster data db UID db= cdDB=63343433
database cluster not initialized
our db requested role is standby followedDB=2a87ea79
running pg_basebackup
sync succeeded
starting database
postgres parameters changed, reloading postgres instance
reloading database configuration
our db requested role is standby followedDB=2a87ea79
already standby
```

### Simulate master death

You can now try killing the actual keeper managing the master postgres instance (if killed with SIGKILL it'll leave an unmanaged postgres instance, if you terminate it with SIGTERM it'll also stop the postgres instance before exiting) (if you just kill postgres, it'll be restarted by the keeper usually before the sentinels declares it as not healty) noticing that the sentinels will:

* declare the master as not healty.
* elect the standby as the new master.
* Remove the proxyconf. The current `psql` connection will be dropped.
* Wait for the new master to be ready.
* Update the proxyconf with the new master address.


```
no keeper info available db=2a87ea79 keeper=postgres0
no keeper info available db=2a87ea79 keeper=postgres0
master db is failed db=2a87ea79 keeper=postgres0
trying to find a standby to replace failed master
electing db as the new master db=63343433 keeper=postgres1
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
