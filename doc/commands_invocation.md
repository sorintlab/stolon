## Commands invocation

There are 4 commands provided by stolon:

* [stolon-keeper](commands/stolon-keeper.md)
* [stolon-sentinel](commands/stolon-sentinel.md)
* [stolon-proxy](commands/stolon-proxy.md)
* [stolonctl](commands/stolonctl.md)

every command has different options and when called without any option or with `--help` they'll show an help with a description for every subcommand and option. Please take also a look at the various examples to see how the commands' options are used.

An option can be specified in the command line or as an environment variables.

Environment variables are prefixed with a string related to the command and take the name of the options but are UPPERCASE and any dashes are replaced by underscores.

For example for the  `stolon-keeper` `--data-dir` command line option the equivalent environment variable is `STKEEPER_DATA_DIR`


| Command            | Environment variable prefix |
|--------------------|-----------------------------|
| stolon-keeper      | STKEEPER                    |
| stolon-sentinel    | STSENTINEL                  |
| stolon-proxy       | STPROXY                     |
| stolonctl          | STOLONCTL                   |
