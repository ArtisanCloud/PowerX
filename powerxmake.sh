#!/bin/bash

function gen_api {
    goctl api go -api ./api/powerx.api -dir .
    rm powerx.go
    echo "gen-api has been executed successfully."
}

function gen_swagger {
    dir=$1
    for api_file in $dir/*.api; do
        filename=$(basename "$api_file" .api)
        goctl api plugin -plugin goctl-swagger="swagger -filename ${filename}.json" -api $api_file -dir swagger
    done
    echo "gen-swagger has been executed successfully."
}

while true; do
    echo
    echo "gen-api"
    echo "gen-swagger [directory path]"
    echo
    read -p "Please enter your command: " cmd
    case $cmd in
        gen-api) gen_api;;
        gen-swagger*)
            dir=$(echo $cmd | cut -d " " -f2)
            gen_swagger $dir
            ;;
        *) echo "Invalid command. Please try again.";;
    esac
done
