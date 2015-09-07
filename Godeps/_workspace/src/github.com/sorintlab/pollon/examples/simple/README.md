# pollon simple example

This example will open a listening tcp4 socket on `127.0.0.1:2222` and the continuously check for a file in the current working directory called `conf`

The file should contain only one line with the `$IP:$PORT` to proxy connections to.

For example if you write in it `localhost:22`

```
echo -n "localhost:22" > conf
```

and then

```
ssh -p 2222 localhost
```

you will be connected (if ssh is running on localhost) to the ssh server listening on `localhost:22`


if you then delete the `conf` or update the address the proxy will disconnect you and future connections will redirect to the new address (or instantly close the connection if the conf file is empty/wrong).
