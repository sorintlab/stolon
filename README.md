# stolon - PostgreSQL cloud native manager

[![Build Status](https://semaphoreci.com/api/v1/projects/fb01aecd-c3d5-407b-a157-7d5365e9e4b6/565617/badge.svg)](https://semaphoreci.com/sorintlab/stolon)

stolon is a cloud native postgreSQL manager for postgreSQL high availability. It's cloud native because it'll let you keep an high available postgreSQL inside your containers (kubernetes integration) but also on every other kind of infrastructure (cloud IaaS, old style infrastructures etc...)

## Features

* leverages postgreSQL streaming replication
* works inside kubernetes letting you handle persistent high availability
* uses [etcd](https://github.com/coreos/etcd) as an high available data store and for leader election

## Architecture

Stolon is composed of 3 main components

* keeper: it manages a postgreSQL instance converging to the clusterview provided by the sentinel(s).
* sentinel: it discovers and monitors members (keepers) and calculates the optimal clusterview.
* proxy: the client's access point. It enforce connections to the right postgreSQL master and forcibly closes connections to unelected masters.

![Stolon architecture](doc/architecture_small.png)

## Requirements

* PostgreSQL >= 9.4
* etcd >= 2.0


## build

```
./build
```

## Simple cluster

This example assumes a running etcd server on localhost

Note: under ubuntu the `initdb` command is not provided in the path. You should updated the exported `PATH` env variable or provide the `--pg-bin-path` command line option to the `stolon-keeper` command.

### Launch first keeper

```
./bin/stolon-keeper --data-dir data/postgres0 --id postgres0 --cluster-name stolon-cluster
```

This will start a stolon keeper with id `postgres0` listening by default on localhost:5431, it will setup and initialize a postgres instance inside `data/postgres0/postgres/`


Now that the first keeper is active we can start a sentinel

```
./bin/stolon-sentinel --cluster-name stolon-cluster
```

Now we can start the proxy

```
./bin/stolon-proxy --cluster-name stolon-cluster --port 25432
```


Connect to the db. Create a test table and do some inserts (we use postgres database for these tests but usually this shouldn't be done).

```
psql --host 127.0.0.1 --port 25432 postgres
```

Now you can start another keeper:

```
./bin/stolon-keeper --data-dir data/postgres1 --id postgres1 --cluster-name stolon-cluster --port 5433 --pg-port 5435
```

This instance will start replicating from the master (postgres0)


## High availability

Stolon tries to be resilent to any partitioning problem. The cluster view is computed by the leader sentinel and is useful to avoid data loss (one example over all avoid that old dead masters coming back are elected as the new master).

There can be tons of different partitioning cases. The primary ones are covered (and in future more will be added) by various [integration tests](tests/integration)
