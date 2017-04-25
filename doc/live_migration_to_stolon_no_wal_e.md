## Live Migration to Stolon without Wal-E

Leveraging Stolon's Point In Time Recovery ([pitr](pitr.md)) feature, it is possible to do a live migration of a legacy Postgresql database to Stolon

This example shows how to have Stolon to execute [pg_basebackup](https://www.postgresql.org/docs/9.5/static/app-pgbasebackup.html) and then start using this latest backup

NOTE: This assumes no client traffic to the Legacy Posgresql database during migration.  A future example may include incremental/wal-e based methods

### Preparing for migration

#### Users

Stolon uses 'stolon' and 'repluser' natively, so these users need to be created on your legacy system before proceeding.  We're assuming the legacy system's superuser is called 'postgres' here. Let's create the 'stolon' superuser, with replication privileges (required for the initial pg_basebackup)

##### stolon

```
createuser -s --replication -U postgres -h legacydatabasehost -P stolon  
```

This will ask twice for the 'stolon' users password (which should match what you have configured for the Secret that your Stolon deployment uses), and then the legacy system's 'postgres' superuser password.

##### repluser

```
createuser --replication -U postgres -h legacydatabasehost -P repluser
```

As for the 'stolon' user, this will ask for the appropriate passwords, which should match the Secret you configured.

#### Configure legacy postgresql.conf for replication (if necessary)

If you have not already configured your legacy Postgresql server for replication, you will need to set a few parameters in your postgresql.conf to enable the replication that pg_basebackup will use

```
wal_level = hot_standby         # required when stolon starts up in same mode
max_wal_senders = 3             # max number of walsender processes
max_replication_slots = 3       # max number of replication slots
```

If these were not already present, you will need to restart your legacy server before proceeding

#### Determine custom Legacy postgresql.conf parameters

Often you will have tweaked your legacy postgresql server for various reasons (ie. 'max_prepared_transactions' or a higher 'max_connections'). Take some time to find what these are, and we will use these when setting up Stolon, since likely these are not set by default by the Docker Postgres image you or Stolon.  Stolon's Keeper will be unable to start Postgresql if these do not match at startup.  Depending on the legacy system's OS distribution, postgresql.conf may or may not be shipped along with the 'pg_basebackup' command, so knowing ahead of time what these are can save you some time.

NOTE: Stolon does support merging postgresql.conf on startup if is provided in the pg_basebackup

#### Postgresql logging

Some versions of Postgresql and official Docker versions do not appear enable file logging by default on startup, so you may want to consider setting them explicitly to help debug in case of issues

```
log_destination = stderr
logging_collector = on
log_directory = /tmp/
```

Leaving the rest as defaults. We're using  /tmp/ here because if the 'pg_basebackup' fails, the Stolon Keeper erases the PGDATA dir and tries again, taking the default location for log files with it.

#### Match postgresql versions

Ensure that Stolon's version of Postgresql matches the legacy Postgresql database version, as a binary copy may not be compatible with differing versions.

#### Application Configuration

Assuming you are using Kubernetes for your entire application stack, prepare the appropriate client connection configuration parameters to have them ready for update in Kubernetes, and then to perform an application restart once Stolon is up and running with the migrated database.  You can apply any ConfigMaps or Secrets pertaining to the Stolon connection now, but do not restart any pods until the migration is complete

### Perform the migration

#### Caution

Since this method is meant to go against a live produciton database, a few 'dry' runs are recommended, where you do not shut down any writes from the legacy application, ensuring that you have everything configured properly.  It is recommended to fully reset Stolon after each dry run (taking care to remove the Keepers and PersistentVolumeClaims inbetween runs)

#### Stop writes to legacy database

As mentioned in the description, you will need to ensure there are no writes going to the system before executing this.  Since 'pg_basebackup' can do a binary copy of a running database relatively quickly, there is not much downtime required.

The author of this document used this method to perform a live migration with under under 5 minutes of downtime against a 16GB Postgresql database, during off peak hours.

#### Monitor the migration

Determine which Keeper is the master Keeper, and you can monitor this progress by 'kubectl exec' into the keeper pod and watching the process, the /stolon-data (PGDATA) directory, or disk usage. If you have configured the postgresql Server for logging to STDERR, you should be able to see any issues in the 'kubectl logs' output 

#### Execute the migration

Now let's call stolonctl to configure the Point In Time Recovery with the pg_basebackup command.  It is assumed you have the $STOLON_CLUSTER_NAME, $STOLON_STORE_BACKEND, $STOLON_STORE_ENDPOINTS environment variables set for your Stolon installation

```
stolonctl --cluster-name=${STOLON_CLUSTER_NAME} --store-backend=${STOLON_STORE_BACKEND} --store-endpoints=${STOLON_STORE_ENDPOINTS} init '{"initMode":"pitr","pitrConfig":{"dataRestoreCommand":"PGPASSWORD=legacydbpassword pg_basebackup -D /stolon-data/postgres/ -P -h legacydbhost -U stolon -x", "archiveRecoverySettings":{"restoreCommand": "echo 1"}}}'
```
Here, since we are doing a complete backup and restore of all databasefiles and existing WAL files (-x option), the restoreCommand merely needs to succeed, hence the 'echo 1'.

Now let's patch in desired postgresql.conf parameters using stolonctl update.  In this example, we also tweak the various Stolon timeouts for checking health, and performing failovers in one command:

```
stolonctl --cluster-name=${STOLON_CLUSTER_NAME} --store-backend=${STOLON_STORE_BACKEND} --store-endpoints=${STOLON_STORE_ENDPOINTS} update --patch '{"failInterval":"10s","requestTimeout":"5s","sleepInterval":"3s","pgParameters":{"max_prepared_transactions":"100","max_conections":"1000"}}
```

Ideally these two commands should run right after each other, so Stolon is aware of what to provide Postgresql when it starts up.


#### Restart Application

Since the new configuration using 'stolon' user and the Stolon proxy has been configured, you may now restart the application (kubectl delete pods, kubectl patch method for rolling restart, or delete/create your Kubernetes Deployment)

#### Handling 

If something goes wrong you can see the errors in the keeper's logs or in postgresql log (if these are related to the archive restore step) and you can retrigger a new pitr reinitializing the cluster.

### Kubernetes tips

The author of this method has configured a Kubernetes Job to perform the migration:

```
apiVersion: batch/v1
kind: Job
metadata:
  name: my-db-replicateinit
spec:
  template:
    metadata:
      name: my-db-stolonctl-replicateinit
    spec:
      containers:
        -
          name: my-db-replicateinit
          image: gcr.io/private_repo/stolon:0.5.0-pg9.4.5
          imagePullPolicy: Always
          env:
            - name: STOLON_CLUSTER_NAME
              valueFrom:
                configMapKeyRef:
                  name: my-db-config
                  key: stolonclustername
            - name: STOLON_STORE_ENDPOINTS
              valueFrom:
                configMapKeyRef:
                  name: my-db-config
                  key: stolonstoreendpoints
            - name: STOLON_STORE_BACKEND
              valueFrom:
                configMapKeyRef:
                  name: my-db-config
                  key: stolonstorebackend
          command:
            - "/bin/bash"
            - "-ec"
            - |
             /usr/bin/yes yes | /usr/local/bin/stolonctl --cluster-name=${STOLON_CLUSTER_NAME} --store-backend=${STOLON_STORE_BACKEND} --store-endpoints=${STOLON_STORE_ENDPOINTS} init '{"initMode":"pitr","pitrConfig":{"dataRestoreCommand":"PGPASSWORD=legacydbpassword pg_basebackup -D /stolon-data/postgres/ -P -h legacydbhost -U stolon -x", "archiveRecoverySettings":{"restoreCommand": "echo 1"}}}' 2>&1
             /usr/local/bin/stolonctl --cluster-name=${STOLON_CLUSTER_NAME} --store-backend=${STOLON_STORE_BACKEND} --store-endpoints=${STOLON_STORE_ENDPOINTS} update --patch '{"failInterval":"10s","requestTimeout":"5s","sleepInterval":"3s","pgParameters":{"max_prepared_transactions":"100","max_connections":"1000"}}'
          securityContext:
            capabilities:
              drop:
                - ALL
      restartPolicy: Never
      imagePullSecrets:
        -
          name: my-docker-key
```

Where we can re-use these ConfigMap keys in the Kubernetes Deployment and other jobs if needed

## ERRATA / TIPS

Improvements to this method are welcome.  Please submit a Pull Request with updates
