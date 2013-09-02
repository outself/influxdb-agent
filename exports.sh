#!/usr/bin/env bash

work=$(python -c 'import os, sys;print os.path.abspath(os.path.dirname(os.path.realpath(sys.argv[1])))' $BASH_SOURCE)
export GOPATH=$work/
