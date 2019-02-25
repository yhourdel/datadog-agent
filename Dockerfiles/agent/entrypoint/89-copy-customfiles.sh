#!/bin/bash

# Copy the custom checks and confs in the /etc/datadog-agent folder
find /conf.d -name '*.yaml' | while read line; do
  echo "'$line' -> '/etc/datadog-agent$line'"
  perl -p -e 's/\$\{(\w+)\}/(exists $ENV{$1}?$ENV{$1}:"")/eg' < "$line" > "/etc/datadog-agent$line"
done

find /checks.d -name '*.py' -exec cp --parents -fv {} /etc/datadog-agent/ \;
