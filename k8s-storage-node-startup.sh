#!/usr/bin/env sh

if [ -z "$NODE_ID" ]; then
  export NODE_ID=$(echo "${HOSTNAME}" | awk -F '-' '{print $NF}')
fi

/app/bin/storage_node "${STORAGE_NODE_CONFIG_PATH}"
