#!/bin/bash

echo "[VER] ------------------------------------------------------"

# shellcheck disable=SC2162
read -p "[VER] enter new version(eg. 1.3.14):" number

if [[ "$OSTYPE" == "linux"* ]] || [[ "$OSTYPE" == "msys"* ]] ; then
    echo "[VER] use sed"
    sed -i 's/actorgo v[0-9.]*/actorgo v'${number}'/' **/go.mod
elif [[ "$OSTYPE" == "darwin"* ]]; then
    echo "[VER] use gsed"
    sed -i 's/actorgo v[0-9.]*/actorgo v'${number}'/' **/go.mod
fi