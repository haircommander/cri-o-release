#!/bin/bash

set -x

version="$1"

if [ -z $version ]; then
	echo Please specify a version
	exit 1
fi
package_name="cri-o"
prefix=devel:kubic:libcontainers:stable:$package_name

allcrioproj=$(osc ls | grep "devel:kubic:libcontainers:stable:$package_name")

fields=$(echo $version | tr '.' " ")
major=$(awk '{ printf $1 }' <<< "$fields" )
minor=$(awk '{ printf $2 }' <<< "$fields" )
patch=$(awk '{ printf $3 }' <<< "$fields" )

# TODO verify versions here
# TODO verify there are no extra fields

if [ "$patch" == "0" ]; then
	oldproj=$(echo "$allcrioproj" | grep $major.$(($minor-1)) | tail -n1)
	newproj=$prefix:$major.$minor
else
	oldproj=$(echo "$allcrioproj" | grep $major.$minor | grep -v $major.$minor.$((patch-1)) | sort -r | tail -n1)
	newproj=$prefix:$major.$minor:$major.$minor.$patch
fi

echo branching $newproj from $oldproj

function create_project() {
	# now, create the new project
	# copy the old project meta data
	OLDMETA_FILE=/tmp/oldmeta
	osc meta prj "$oldproj" > "$OLDMETA_FILE"

	# update the old meta file
	sed -i 's/project name=".*">/project name="'"$newproj"'">/g' $OLDMETA_FILE

	# create the new project
	osc meta prj "$newproj" -F "$OLDMETA_FILE"
}


function update_prjconf() {
	# create temp prjconf to add
	PRJCONF_FILE=/tmp/prjconf
	cat<<EOF > $PRJCONF_FILE
Release: <CI_CNT>.<B_CNT>%%{?dist}
%if "%_repository" == "CentOS_8" || "%_repository" == "CentOS_8_Stream"
ExpandFlags: module:go-toolset-rhel8
%endif
%if "%_repository" == "CentOS_8_Stream"
Prefer: centos-stream-release
%endif
EOF

	osc meta prjconf  -F "$PRJCONF_FILE" "$newproj"
}

function copy_packages() {
	# copy the old project to the new
	# skip the cri-o package, we'll make a new one
	for oldpac in $(osc ls $oldproj | grep -v "$package_name"); do
		osc branch "$oldproj" "$oldpac" "$newproj"
	done
#	if [ "$patch" != "0" ]; then
#		osc branch "$oldproj" "$package_name" "$newproj"
#	fi
}

create_project
update_prjconf
copy_packages
