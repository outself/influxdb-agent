#!/usr/bin/env bash

cd `dirname $0`

export GOPATH=`pwd`

go get github.com/errplane/errplane-go

