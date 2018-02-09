# README #
To do initial setup
- install Glide - https://glide.sh/

- install Make

- Clone repo, install dependencies, build
```
cd $GOPATH
mkdir -p src
cd src
git clone git@bitbucket.org:alexandru_ionica/cloudbackup.git
cd cloudbackup
make deps
make
```