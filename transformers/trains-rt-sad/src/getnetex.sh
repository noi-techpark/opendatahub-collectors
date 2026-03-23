#!/bin/bash

# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FTP_HOST="ftp.sta.bz.it"
FTP_DIR="/netex/$(date +%Y)/plan/allTrains"

# Get the most recent *.xml file
LATEST=$(lftp "$FTP_HOST" <<EOF
cd $FTP_DIR
cls -1 --sort=date *.xml | head -1
EOF
)

if [ -z "$LATEST" ]; then
  echo "No XML files found"
  exit 1
fi

echo "Downloading: $LATEST"

lftp "$FTP_HOST" <<EOF
cd $FTP_DIR
get "$LATEST" -o "netex.new.xml"
EOF
mv netex.new.xml netex.xml