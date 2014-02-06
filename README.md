# agent

## Building

`./build.sh` should take care of building the binary and installing the go dependencies. If ran with `UPDATE=on` env variable
the script will update the dependencies, i.e. run `go get -u` instead of just `go get`.

## Building for 386 on x86_64

This is just for informational purposes, you're not expected to run these commands.

### Compile Go for 386

Run the following

```
cd $GOROOT/src
GOARCH=386 ./make.bash --no-clean
```

### Build the agent for 386 on x86_64

`GOARCH=386 ./build.sh`

## Packaging

`./package.sh major.minor.patch` will generate .deb files in out_rpm.

The script uses fpm to create the rpm and debian packages, on ubuntu you'll need `rpmbuild` which
you can install by running `sudo apt-get install rpm`

## Installing

Create an errplane user using the following instruction:

`useradd -r errplane`

To install the package run

`sudo dpkg -i errplane-agent_0.1.0_amd64.deb`

Generate a config if this is the first time to install the agent

`sudo -u errplane errplane-config-generator -api-key your-api-key -app-key your-app-key -environment production`

An init.d script will be installed to start and stop the agent `/etc/init.d/errplane-agent`
