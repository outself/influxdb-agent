#!/usr/bin/env bash

cd `dirname $0`

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version.number>"
    exit 1
fi

# Load RVM into a shell session *as a function*
if [ -s "$HOME/.rvm/scripts/rvm" ]; then
  # First try to load from a user install
  source "$HOME/.rvm/scripts/rvm"
elif [ -s "/usr/local/rvm/scripts/rvm" ]; then
  # Then try to load from a root install
  source "/usr/local/rvm/scripts/rvm"
else
  printf "ERROR: An RVM installation was not found.\n"
fi

rvm use --create 1.9.3@errplane-agent
gem install fpm
version=$1
data_dir=out_rpm/data/errplane-agent/versions/$version/
config_dir=out_rpm/etc/errplane-agent
initd_dir=out_rpm/etc/init.d/
plugins_dir=out_rpm/data/errplane-agent/plugins
shared_dir=out_rpm/data/errplane-agent/shared

rm -rf out_rpm
mkdir -p $data_dir $initd_dir $config_dir $log_dir $plugins_dir $shared_dir
rm -rf package
mkdir package

cp scripts/post_install.sh /tmp/post_install.sh
sed -i "s/REPLACE_VERSION/${version}/g" /tmp/post_install.sh

#cleanup first

rm errplane-agent*.rpm
rm errplane-agent*.deb

function copy_files {
    cp agent $data_dir/
    cp scripts/errplane-agent-daemon $data_dir/
    cp scripts/agent_ctl $data_dir/
    cp config-generator $data_dir/
    cp sudoers-generator $data_dir/
    cp opensource.md $data_dir/
    cp scripts/init.d.sh $initd_dir/errplane-agent
}

# build the x86_64 version
UPDATE=on ./build.sh -v $version || exit 1
copy_files
pushd out_rpm
fpm  -s dir -t rpm --rpm-user errplane --deb-group errplane --after-install /tmp/post_install.sh -n errplane-agent -v $version . || exit $?
mv *.rpm ../package/
fpm  -s dir -t deb --deb-user errplane --deb-group errplane --after-install /tmp/post_install.sh -n errplane-agent -v $version . || exit $?
mv *.deb ../package/
popd

rm -rf out_rpm
mkdir -p $data_dir $initd_dir $config_dir $log_dir $plugins_dir $shared_dir

# build the 32 bit version
GOARCH=386 UPDATE=on ./build.sh -v $version || exit 1
copy_files
pushd out_rpm
setarch i386 fpm -s dir -t rpm --rpm-user errplane --rpm-group errplane --after-install /tmp/post_install.sh -n errplane-agent -v $version . || exit $?
mv *.rpm ../package/
fpm -s dir -t deb -a i386 --deb-user errplane --deb-group errplane --after-install /tmp/post_install.sh -n errplane-agent -v $version . || exit $?
mv *.deb ../package/
popd

