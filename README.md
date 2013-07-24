# agent

## Building

`./build.sh` should take care of building the binary and installing the go dependencies. If ran with `UPDATE=on` env variable
the script will update the dependencies, i.e. run `go get -u` instead of just `go get`.

## Building for different architectures

### Compiling Go for 386

Run the following

```
cd $GOROOT/src
GOARCH=386 ./make.bash --no-clean
```

### Building for 386 on x86_64

`GOARCH=386 ./build.sh`

## Packaging

`./package.sh major.minor.update` will generate .deb files in out_rpm.

## Installing

Create an errplane user using the following instruction:

`useradd -r errplane`

To install the package run

`sudo dpkg -i errplane-agent_0.1.0_amd64.deb`

If this is the first time to install the package, edit `/etc/errplane-agent/config.yml`.

An init.d script will be installed to start and stop the agent `/etc/init.d/errplane-agent`
