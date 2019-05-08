#!/usr/bin/env bash

which dep > /dev/null
if [[ $? -ne 0 ]]; then
  echo "Please install glide to continue"
  echo "https://glide.sh/"

  exit 1
fi

glide install
