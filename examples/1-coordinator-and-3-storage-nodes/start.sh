#!/usr/bin/env sh

./bin/storage_node ./examples/1-coordinator-and-3-storage-nodes/storage_node_1.yml &
./bin/storage_node ./examples/1-coordinator-and-3-storage-nodes/storage_node_2.yml &
./bin/storage_node ./examples/1-coordinator-and-3-storage-nodes/storage_node_3.yml &

./bin/coordinator ./examples/1-coordinator-and-3-storage-nodes/coordinator.yml