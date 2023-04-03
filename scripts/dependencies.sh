#!/bin/bash -eu

# If CI variable is set, and is equal to true we are in a CI environment
IS_CI=false
if [ -n "${CI+x}" ] && [ "$CI" = true ]; then
  IS_CI=true
fi
echo "var: IS_CI = $IS_CI"

# If CI is true, and we have both the vendor and bin directories, then the cache is already present
HAS_CACHE=false
if $IS_CI && [ -d "./vendor" ] && [ -d "./bin" ]; then
  HAS_CACHE=true
fi
echo "var: HAS_CACHE = $HAS_CACHE"

# If CI run mod vendor, otherwise run mod download
if $IS_CI; then
  # If vendor is already cached, do nothing
  if $HAS_CACHE; then
    echo "cache: go mod vendor"
  else
    echo "run: go mod vendor"
    go mod vendor
    (cd tools && go mod vendor)
  fi
else
  echo "run: go mod download"
  go mod download
fi

# Install the tools from tools.go, or nothing if CI & tools already cached
if $IS_CI && $HAS_CACHE; then
  echo "cache: go install tools"
else
  # Parses out the imports in the tools.go file to then install the binaries
  lintTools=()
  while IFS='' read -r line; do lintTools+=("$line"); done < <(sed -En 's/[[:space:]]+_ "(.*)"/\1/p' tools/tools.go)
  for tool in "${lintTools[@]}"; do
    echo "run: go install tool ($tool)"
    (cd tools && go install "$tool")
  done
fi

# Run vendor for CI and tidy for other environments
if ! $IS_CI; then
  echo "run: go mod tidy"
  go mod tidy
fi
