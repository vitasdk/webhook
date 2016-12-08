#!/bin/sh
echo "triggering $1"

if [ ! -d "$1" ]; then
    mkdir -p "$(echo "$1" | cut -d/ -f1)"
    git clone "git@github.com:$1.git" "$1"
else
    cd "$1"
    git pull
    cd -
fi

cd "$1"

git config user.email "vitasdk@henkaku.xyz"
git config user.name "Auto Builder"

echo "commiting"
git commit --allow-empty -m "Build on $(date)"
echo "push"
git push origin master
