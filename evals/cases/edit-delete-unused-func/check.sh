#!/usr/bin/env bash
! grep -q old_helper utils.py && grep -q 'def greet' utils.py && grep -q 'def shout' utils.py && [ "$(python3 main.py)" = "HI BOB!" ]
