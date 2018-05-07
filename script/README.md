# Boilerplate

This repo contains a lot of boilerplate configuration used to create Go applications.
This small script turns `README.md` files from `cmd/<name>/README.md` into man pages.
All you have to do is create a file similar to the one in this repo and run `make man`.
This also requires the boilerplate `Makefile`. Change a few variables at the top and
it mostly just works.

You'll need to install `ronn` to make this script work. This seems to do it:

```shell
sudo -H gem install ronn --no-ri --no-rdoc
```

Maybe one day we'll get a working `go-ronn` application.
