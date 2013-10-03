#!/usr/bin/env bash

cd `dirname $0`

set -e

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
data_dir=out_rpm/data/anomalous-agent/versions/$version/
config_dir=out_rpm/etc/anomalous-agent
initd_dir=out_rpm/etc/init.d/
shared_dir=out_rpm/data/anomalous-agent/shared
plugins_dir=$shared_dir/plugins
custom_plugins_dir=$shared_dir/custom-plugins

# create the package files
rm -rf out_rpm
mkdir -p $data_dir $initd_dir $config_dir $log_dir $shared_dir $plugins_dir $custom_plugins_dir

# replace the version with the version that we're building
cp scripts/post_install.sh /tmp/post_install.sh
sed -i "s/REPLACE_VERSION/${version}/g" /tmp/post_install.sh
chmod a+x /tmp/post_install.sh
cp scripts/pre_install.sh /tmp/

function package_files {
    if [ $# -ne 1 ]; then
        echo "Usage: $0 architecture"
        return 1
    fi
    rm -rf package/anomalous-agent
    mkdir -p package/anomalous-agent
    pushd package
    cp ../agent anomalous-agent/
    cp ../sudoers-generator anomalous-agent/
    cp ../config-generator anomalous-agent/
    cp ../opensource.md anomalous-agent/
    cp ../scripts/init.d.sh anomalous-agent/
    cp ../scripts/anomalous-agent-daemon anomalous-agent/
    cp /tmp/post_install.sh anomalous-agent/
    echo -n $version > anomalous-agent/version
    tar -cvzf anomalous-agent_${version}_$1.tar.gz anomalous-agent
    popd
}

function build_packages {
    if [ $# -ne 1 ]; then
        echo "Usage: $0 architecture"
        return 1
    fi

    if [ $1 == "386" ]; then
        rpm_args="setarch i386"
        deb_args="-a i386"
    fi

    rm -rf out_rpm
    mkdir -p out_rpm/data/anomalous-agent/versions/$version
    cp -r package/anomalous-agent/* out_rpm/data/anomalous-agent/versions/$version
    pushd out_rpm
    $rpm_args fpm  -s dir -t rpm --rpm-user anomalous --deb-group anomalous --pre-install /tmp/pre_install.sh --after-install /tmp/post_install.sh -n anomalous-agent -v $version . || exit $?
    mv *.rpm ../package/
    fpm  -s dir -t deb $deb_args --deb-user anomalous --deb-group anomalous --pre-install /tmp/pre_install.sh --after-install /tmp/post_install.sh -n anomalous-agent -v $version . || exit $?
    mv *.deb ../package/
    popd
}

# bulid and package the x86_64 version
UPDATE=on ./build.sh -v $version && package_files amd64 && build_packages amd64 && \
    CGO_ENABLED=1 GOARCH=386 UPDATE=on ./build.sh -v $version && package_files 386
