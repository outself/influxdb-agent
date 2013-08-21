#!/usr/bin/env bash

cd `dirname $0`

modified=$(git ls-files --modified | wc -l)

if [ $modified -ne 0 ]; then
    echo "Please commit or stash all your changes and try to run this command again"
    exit 1
fi

if [ $# -ne 1 ]; then
    current_version=`git tag | sort -V | tail -n1`
    current_version=${current_version#v}
    version=`echo $current_version | awk 'BEGIN {FS="."}; {print $1 "." $2 "." ++$3}'`
else
    version=$1
fi

if [ "x$assume_yes" != "xtrue" ]; then
    echo -n "Release version $version ? [Y/n] "
    read response
    response=`echo $response | tr 'A-Z' 'a-z'`
    if [ "x$response" == "xn" ]; then
        echo "Aborting"
        exit 1
    fi
fi

echo "Releasing version $version"

if ! which aws > /dev/null 2>&1; then
    echo "Please install awscli see https://github.com/aws/aws-cli for more details"
    exit 1
fi

if ! ./package.sh $version; then
    echo "Build failed. Aborting the release"
    exit 1
fi

current_branch=`git branch --no-color | grep '*' | cut -d' ' -f2`
git tag v$version
git push origin $current_branch
git push origin --tags

for filepath in package/*.{rpm,deb}; do
    [ -e "$filepath" ] || continue
    echo "Uploading $filepath to S3"
    filename=`basename $filepath`
    AWS_CONFIG_FILE=~/aws.conf aws s3 put-object --bucket errplane-agent --key $filename --body $filepath --acl public-read
done
