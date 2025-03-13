# Contributing

All contributions are useful, whether it is a simple typo, a more complex change, or
just pointing out an issue. We welcome any contribution so feel free to take part in
the discussions. If you want to contribute to the project, please make sure to review
this document carefully.

- [Setting up the environment](#setting-up-the-environment)
- [Before submitting a change](#before-submitting-a-change)
- [Your first pull request](#your-first-pull-request)

## Working Environment

### Prerequisites

- Git
- Golang
- One or all of the supported datastore (Zookeeper / Consul / Etcd / Redis / BoltDB)

### Installing Golang

Install golang using your favorite package manager on Linux or with the archive
following these [Guidelines](https://golang.org/doc/install).

An easy way to get started on mac OS is to use [homebrew](https://brew.sh) and type
`brew install go` in a shell.

### Local testing of key/value stores

In addition to installing golang, you will need to install some or all of the key value stores for testing.

Refer to each of these stores documentation in order to proceed with installation.
Generally, the tests are using the **default configuration** with the **default port** to connect to a store and run the test suite.

To test a change, you can run the test suite with the following command:

```bash
make test
```

## Before submitting a change

Make sure you check each of these items before you submit a pull request to avoid
many unnecessary back and forth in github comments (and will help us review and include
the change as soon as possible):

- **Open an issue** to clearly state the problem. This will be helpful to keep track
of what needs to be fixed. This also helps triaging and prioritising issues.

- **Run the following command**: `make validate`, to ensure that your code is properly
formatted.

- **For non-trivial changes, write a test**: this is to ensure that we don't encounter
any regression in the future.

- **Write a complete description** for your pull request (avoid using `-m` flag when
committing a change unless it is a trivial one).

- **Sign-off your commits** using the `-s` flag (you can configure an alias to
`git commit` adding `-s` for convenience).

- **Squash your commits** if the pull requests includes many commits that are related.
This is to maintain a clean history of the change and better identify faulty commits
if reverting a change is ever needed. We will tell you if squashing your commits is
necessary.

- **If the change is solving one or more issues listed on the repository**: you
can reference the issue in your comment with `closes #XXX` or `fixes #XXX`.
This will automatically close related issues on merging the change. See
[Github documentation](https://help.github.com/articles/closing-issues-using-keywords/)
for more details.

Finally, submit your *Pull Request*.

## Your first Pull Request

You made it to your first Pull Request? It's only the start of the process.
Following steps may include a discussion on the design and tradeoffs of your
proposed solution.

Additionaly there will be a *code review process* to find out potential bugs. Part
of being a helpful community is to make sure we point out improvements and deliver
actionable items to work towards fixing potential issues. Feel free to ask questions
if you are stuck so we can help you.

*Don't be discouraged* if your change happens not to be included. All contributions
are helpful in a way. Your PR most certainly made the discussion go forward in many
aspects and helped working towards our common goal of making the project better for
everyone.

**Welcome!**
