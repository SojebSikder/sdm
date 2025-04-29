# Description
Fast Internet download manager created using Go.

It downloads faster by downloading chunks parallelly

# Screenshots
![screenshot1](./screenshots/screenshot1.png)

# Build
```
./build.sh
```

# Usage
```
sdm download <url> --output <location>
```

# Supported commands

- `download` - for downloading file 
    - (optional) support `--output` flag that used to specify the output location
    - (optional) `--worker` flag to override the worker count

