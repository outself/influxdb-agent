#!/usr/bin/env bash

work=$(python -c 'import os, sys;print os.path.abspath(os.path.dirname(os.path.realpath(sys.argv[1])))' $0)
export GOPATH=$work/
leveldb_dir=/tmp/leveldb
snappy_dir=/tmp/snappy
export CGO_CFLAGS="-I$leveldb_dir/include"
export CGO_LDFLAGS="$leveldb_dir/libleveldb.a $snappy_dir/.libs/libsnappy.a -lstdc++"
