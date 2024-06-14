#!/usr/bin/env sh


set -e

# check to see if this file is being run or sourced from another script
_is_sourced() {
	# https://unix.stackexchange.com/a/215279
	[ "${#FUNCNAME[@]}" -ge 2 ] \
		&& [ "${FUNCNAME[0]}" = '_is_sourced' ] \
		&& [ "${FUNCNAME[1]}" = 'source' ]
}

# check arguments for an option that would cause /svalinn to stop
# return true if there is one
_want_help() {
	local arg
	for arg; do
		case "$arg" in
			-'?'|--help|-v)
				return 0
				;;
		esac
	done
	return 1
}

_main() {
	# if command starts with an option, prepend gungnir
	if [ "${1:0:1}" = '-' ]; then
		set -- /gungnir "$@"
	fi
		# skip setup if they aren't running /gungnir or want an option that stops /gungnir
	if [ "$1" = '/gungnir' ] && ! _want_help "$@"; then
		echo "Entrypoint script for Gungnir Server ${VERSION} started."

		if [ ! -s /etc/gungnir/gungnir.yaml ]; then
		  echo "Building out template for file"
		  /spruce merge --prune service.fixed /gungnir.yaml /tmp/gungnir.yaml > /etc/gungnir/gungnir.yaml
		fi
	fi

	exec "$@"
}

# If we are sourced from elsewhere, don't perform any further actions
if ! _is_sourced; then
	_main "$@"
fi