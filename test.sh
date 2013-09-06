#!/usr/bin/env bash

cd `dirname $0`
. exports.sh

go get launchpad.net/gocheck

function print_usage {
    echo "  -o|--only: Run the test that matches the given regex"
}

TEMP=`getopt -o ho: --long help,only: \
     -n $0 -- "$@"`

if [ $? != 0 ] ; then print_usage ; exit 1 ; fi

# Note the quotes around `$TEMP': they are essential!
eval set -- "$TEMP"

while true ; do
    case "$1" in
        -h|--help) print_usage; exit 1; shift;;
        -o|--only) regex=$2; shift 2;;
        --) shift ; break ;;
        *) echo "Internal error!" ; exit 1 ;;
    esac
done

if [ "x$regex" != "x" ]; then
    gocheck_args="-gocheck.f $regex"
fi

go test -v apps/agent $gocheck_args
go test -v datastore $gocheck_args
