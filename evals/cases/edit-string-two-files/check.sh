#!/usr/bin/env bash
! grep -qi acme index.html about.txt && grep -q 'Zenith Home' index.html && grep -q 'Welcome to Zenith' index.html && grep -q 'Zenith was founded in 2010' about.txt && grep -q hello@example.com about.txt
