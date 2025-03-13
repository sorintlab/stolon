# pollon - Simple dynamic tcp proxy library

pollon is a simple dynamic tcp proxy library that retrieves the backend destination ip:address from a user defined function and, if it's changed, forcibly closes all connections to the old destination.


# simple usage

See the [simple example](examples/simple)


# See also
[stolon](https://github.com/sorintlab/stolon) Stolon uses pollon for its `stolon-proxy` to enforce that the connections are directed to the right master
