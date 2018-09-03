## Forcing a failover

You can force a "master" keeper failover using the [stolonctl failkeeper](commands/stolonctl_failkeeper.md)

This commands forces a keeper as "temporarily" failed. It's just a one shot operation, the sentinel will compute a new clusterdata considering the keeper as failed and then restore its state to the real one.

For example, if the force failed keeper is a master, the sentinel will try to elect a new master. If no new master can be elected, the force failed keeper, if really healthy, will be re-elected as master

To avoid losing any transaction when using asynchronous replication take a look at this recipe:

* [Manual switchover without transactions loss](manual_switchover.md)
