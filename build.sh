#!/usr/bin/env bash

. exports.sh

leveldb_version=1.12.0
snappy_version=1.1.0

build_args=""
if [ "$UPDATE" = "on" ]; then
    build_args="-u"
fi

snappy_dir=/tmp/snappy
snappy_file=snappy-$snappy_version.tar.gz
if [ ! -d $snappy_dir -o ! -e $snappy_dir/$snappy_file -o ! -e $snappy_dir/.libs/libsnappy.a ]; then
    rm -rf $snappy_dir
    mkdir -p $snappy_dir
    pushd $snappy_dir
    wget https://snappy.googlecode.com/files/$snappy_file
    tar --strip-components=1 -xvzf $snappy_file
    ./configure
    make
    popd
fi

leveldb_dir=/tmp/leveldb
leveldb_file=leveldb-$leveldb_version.tar.gz
if [ ! -d $leveldb_dir -o ! -e $leveldb_dir/$leveldb_file -o ! -e $leveldb_dir/libleveldb.a ]; then
    rm -rf $leveldb_dir
    mkdir -p $leveldb_dir
    pushd $leveldb_dir
    wget https://leveldb.googlecode.com/files/$leveldb_file
    tar --strip-components=1 -xvzf $leveldb_file
    CXXFLAGS="-I/tmp/snappy" LDFLAGS="-L/tmp/snappy/.libs" make
    popd
fi

git submodule update --init

go get $build_args github.com/errplane/errplane-go \
    github.com/errplane/gosigar \
    launchpad.net/goyaml \
    code.google.com/p/log4go \
    github.com/bmizerany/pat \
	  github.com/pmylund/go-cache \
    github.com/howeyc/fsnotify \
    code.google.com/p/goprotobuf/proto \
    code.google.com/p/goprotobuf/protoc-gen-go

rm src/datasotre/*.pb.go
PATH=bin:$PATH protoc --go_out=. src/datastore/*.proto

go build apps/agent
go build apps/config-generator
go build apps/sudoers-generator
