#!/bin/bash

go mod tidy
go build -ldflags "-w -s" -trimpath
