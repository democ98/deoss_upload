#!/bin/bash

cd /home/ubuntu/goCode/src/deoss-upload
nohup go run main.go -filepaths "/home/ubuntu/torrent/downloads/annas-archive-ia-acsm-j.tar" > /home/ubuntu/goCode/src/deoss-upload/upload.log 2>&1 &