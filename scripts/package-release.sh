#!/usr/bin/env bash

bin_dir="bin"

name=$1
version=$2
if [[ -z "$name" || -z "$version" ]]; then
  echo "usage: $0 <provider-name> <version>"
  exit 1
fi

cd $bin_dir

for d in *
do
    zip -r $name-$d'_'$version.zip $d

    if [ $? -ne 0 ]; then
        echo 'An error has occurred! Aborting the script execution...'
        exit 1
    fi
done