#!/bin/bash

OBOT_VOLUMES=""
HOMEVOL="${DATABASE_WORKSPACE_ID}"
if [ -z "$HOMEVOL" ]; then
    HOMEVOL="${GPTSCRIPT_WORKSPACE_ID}"
fi

if [ -n "$HOMEVOL" ]; then
    HOMEVOLNAME="user-$(echo $HOMEVOL | shasum -a 256 | awk '{print $1}')"
    OBOT_VOLUMES="${OBOT_VOLUMES} -v ${HOMEVOLNAME}:/home"
fi

DATADIR=$(echo "$GPTSCRIPT_WORKSPACE_ID" | sed 's!directory://!!')
if [ -d "$DATADIR" ]; then
    # Create the directory now to ensure that it's the correct permissions
    mkdir -p "${DATADIR}/files"
    OBOT_VOLUMES="${OBOT_VOLUMES} -v ${DATADIR}/files:/mnt/data"
fi
