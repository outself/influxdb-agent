#!/usr/bin/env bash

. exports.sh

current_dir=$(readlink -f $(dirname $0))
snappy_patch=$current_dir/leveldb-patches/0001-use-the-old-glibc-memcpy-snappy.patch
leveldb_patch=$current_dir/leveldb-patches/0001-use-the-old-glibc-memcpy-leveldb.patch
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
    # apply the path to use the old memcpy and avoid any references to the GLIBC_2.14
    patch -p1 < $snappy_patch || (echo "Cannot patch this version of snappy" && exit 1)
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
    # apply the path to use the old memcpy and avoid any references to the GLIBC_2.14
    patch -p1 < $leveldb_patch || (echo "Cannot patch this version of leveldb" && exit 1)
    CXXFLAGS="-I/tmp/snappy" LDFLAGS="-L/tmp/snappy/.libs" make
    popd
fi

stdcxx_location=`g++ -print-file-name=libstdc++.a`

git submodule update --init

pushd src/github.com/jmhodges/levigo/
find . -name \*.go | xargs sed -i 's/\/\/ #cgo LDFLAGS: -lleveldb\|#cgo LDFLAGS: -lleveldb//g'
popd

export CGO_CFLAGS="-I$leveldb_dir/include"
export CGO_LDFLAGS="-v $leveldb_dir/libleveldb.a $snappy_dir/.libs/libsnappy.a -lstdc++"

go get $build_args github.com/errplane/errplane-go \
    github.com/errplane/gosigar \
    launchpad.net/goyaml \
    code.google.com/p/log4go \
    github.com/bmizerany/pat \
	  github.com/pmylund/go-cache \
    github.com/howeyc/fsnotify

go build apps/agent
go build apps/config-generator
go build apps/sudoers-generator
