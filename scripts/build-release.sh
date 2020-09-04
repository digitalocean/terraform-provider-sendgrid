#!/usr/bin/env bash

bin_dir="bin"
platforms=(
  "windows/amd64"
  "darwin/amd64"
  "linux/amd64"
)

name=$1
version=$2
if [[ -z "$name" || -z "$version" ]]; then
  echo "usage: $0 <provider-name> <version>"
  exit 1
fi
binary_name=$name'_'$version

for platform in "${platforms[@]}"
do
    platform_split=(${platform//\// })
    GOOS=${platform_split[0]}
    GOARCH=${platform_split[1]}

    output_dir=$bin_dir/$GOOS'_'$GOARCH
    mkdir -p $output_dir

    output_name=$output_dir/$binary_name
    if [ $GOOS = "windows" ]; then
        output_name+='.exe'
    fi

    env GOOS=$GOOS GOARCH=$GOARCH CGO_ENABLED=0 go build -v -o $output_name
    if [ $? -ne 0 ]; then
        echo 'An error has occurred! Aborting the script execution...'
        exit 1
    fi
done