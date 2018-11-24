# Cross compatibility with osx
# origin source: https://github.com/kubernetes/kubernetes/ blob/master/hack/lib/init.sh#L102
function readlinkdashf() {
  # run in a subshell for simpler 'cd'
  (
    if [[ -d "$1" ]]; then # This also catch symlinks to   dirs.
      cd "$1"
      pwd -P
    else
      cd $(dirname "$1")
      local f
      f=$(basename "$1")
      if [[ -L "$f" ]]; then
        readlink "$f"
      else
        echo "$(pwd -P)/${f}"
      fi
    fi
  )
}
