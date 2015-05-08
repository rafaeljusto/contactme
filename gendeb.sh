#!/bin/sh

usage() {
  echo "Usage: $1 <version> <release>"
}

pack_name="contactme"
version="$1"
release="$2"
vendor="Rafael Dantas Justo"
maintainer="Rafael Dantas Justo <adm@rafael.net.br>"
url="http://github.com/rafaeljusto/contactme"
license="MIT"
description="Microservice for sending e-mails via HTTP interface"

if [ -z "$version" ]; then
  echo "Version not defined!"
  usage $0
  exit 1
fi

if [ -z "$release" ]; then
  echo "Release not defined!"
  usage $0
  exit 1
fi

install_path=/usr/local/bin
config_path=/etc/contactme
tmp_dir=/tmp/contactme

workspace=`echo $GOPATH | cut -d: -f1`
workspace=$workspace/src/github.com/rafaeljusto/contactme

# recompiling everything
current_dir=`pwd`
cd $workspace
go build
cd $current_dir

if [ -f $pack_name*.deb ]; then
  # remove old deb
  rm $pack_name*.deb
fi

if [ -d $tmp_dir ]; then
  rm -rf $tmp_dir
fi

mkdir -p $tmp_dir$install_path $tmp_dir$config_path
mv $workspace/contactme $tmp_dir$install_path/
cp $workspace/contactme.yaml $tmp_dir$config_path/

fpm -s dir -t deb \
  --exclude=.git -n $pack_name -v "$version" --iteration "$release" --vendor "$vendor" \
  --maintainer "$maintainer" --url $url --license "$license" --description "$description" \
  --deb-upstart $workspace/contactme.upstart \
  --deb-user root --deb-group root \
  --prefix / -C $tmp_dir usr/local/bin etc/contactme

