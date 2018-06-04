## PostgreSQL SSL/TLS setup

SSL/TLS access to an HA postgres managed by stolon can be configured as usual (see the [official postgres doc](https://www.postgresql.org/docs/current/static/ssl-tcp.html)). The setup is done [defining the required pgParameters inside the cluster spec](postgres_parameters.md). If this is enabled also replication between instances will use tls (currently it'll use the default replication mode of "prefer").

If you want to enable client side full verification (`sslmode=verify-full` in the client connection string) you should configure the certificate CN to contain the FQDN or IP address that your client will use to connect to the stolon proxies. Depending on your architecture you'll have more than one stolon proxies behind a load balancer, a keepealived ip, a k8s service etc... So the certificate CN should be set to the hostname or ip that your client will connect to.

For the above reasons, the certificate, key and root CA files will usually be the same for every postgres instance (primary or standbys) so they can be put inside the PGDATA directory (so they could be automatically copied to new instances during a resync) or also outside it (in this case you should ensure that they exists and are available to the postgres instance).
