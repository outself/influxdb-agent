#!/usr/bin/env bash

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version.number>"
    exit 1
fi

# Load RVM into a shell session *as a function*
if [[ -s "$HOME/.rvm/scripts/rvm" ]] ; then
  # First try to load from a user install
  source "$HOME/.rvm/scripts/rvm"
elif [[ -s "/usr/local/rvm/scripts/rvm" ]] ; then
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
log_dir=out_rpm/data/errplane-agent/shared/log
initd=out_rpm/etc/init.d/

rm -rf out_rpm
mkdir -p $data_dir $initd $config_dir $log_dir

cp agent $data_dir/
cp sample_config.yml $data_dir

cp scripts/post_install.sh /tmp/post_install.sh
sed -i "s/REPLACE_VERSION/${version}/g" /tmp/post_install.sh

#cleanup first

rm errplane-agent*.rpm
rm errplane-agent*.deb
pushd out_rpm
fpm  -s dir -t rpm --after-install /tmp/post_install.sh -n errplane-agent -v $version .
fpm  -s dir -t deb --deb-user errplane --deb-group errplane --after-install /tmp/post_install.sh -n errplane-agent -v $version .
popd
