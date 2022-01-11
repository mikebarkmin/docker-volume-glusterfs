#!/bin/bash

# Test for multiple volumes with same settings.
# If we create multiple volumes with same settings but different names, 
# then there should not be a problem with mounting and unmounting
#
# This situation resulted in completed volume removal in older versions.

docker volume rm stackb_test-volume
docker volume rm stacka_test-volume

cd stacka
docker-compose up -d
# docker-compose exec alpine2 pwd
docker-compose exec alpine2 rm -rf /data/*

docker-compose exec alpine2 sh -c 'echo "test" > test_data'
# docker-compose exec alpine2 cat test_data
# docker-compose exec alpine2 ls -l

cd ../stackb 

docker-compose up -d # This should crash if not fixed
docker-compose down 
docker volume rm stackb_test-volume # This should cause complete volume prune

cd ../stacka
docker-compose exec alpine2 sh -c 'if [ -f "test_data" ]; then echo "TEST SUCCESSFULL"; else echo "TEST FAIL"; fi;'
docker-compose down -v

cd ../stackb 
docker-compose down  -v