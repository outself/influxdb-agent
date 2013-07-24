#!/usr/bin/env bash

cd `dirname $0`

modified=$(git ls-files --modified | wc -l)

if [ $modified -ne 0 ]; then
    echo "Please commit or stash all your changes and try to run this command again"
    exit 1
fi

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version.number>"
    exit 1
fi

if ! which aws > /dev/null 2>&1; then
    echo "Please install awscli see https://github.com/aws/aws-cli for more details"
    exit 1
fi

version=$1
if ! ./package.sh $version; then
    echo "Build failed. Aborting the release"
    exit 1
fi

current_branch=`git branch --no-color | grep '*' | cut -d' ' -f2`
git tag v$version
git push origin $current_branch
git push origin --tags

for filepath in out_rpm/*.{rpm,deb}; do
    [ -e "$i" ] || continue
    filename=`dirname $filepath`
    AWS_CONFIG_FILE=~/aws.conf aws s3 put-object --bucket errplane-agent --key $filename --body $filepath
done
