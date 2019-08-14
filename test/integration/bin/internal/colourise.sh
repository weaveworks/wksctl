# NB only to be sourced

colourise() {
    [ -t 0 ] && echo -ne $'\e['$1'm' || true
    shift
    # It's important that we don't do this in a subshell, as some
    # commands we execute need to modify global state
    "$@"
    [ -t 0 ] && echo -ne $'\e[0m' || true
}

whitely() {
    colourise '1;37' "$@"
}

greyly () {
    colourise '0;37' "$@"
}

redly() {
    colourise '1;31' "$@"
}

greenly() {
    colourise '1;32' "$@"
}

