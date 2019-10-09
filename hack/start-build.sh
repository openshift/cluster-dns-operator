#!/bin/bash

set -euo pipefail

oc -n openshift-dns-operator start-build dns-operator \
   ${V+--follow} --wait

if [[ -n "${DEPLOY+1}" ]]
then
    oc -n openshift-dns-operator patch deploy/dns-operator \
       --type=strategic --patch='
{
  "spec": {
    "template": {
      "spec": {
        "containers": [
          {
            "name": "dns-operator",
            "image": "image-registry.openshift-image-registry.svc:5000/openshift-dns-operator/dns-operator:latest",
            "imagePullPolicy": "Always"
          }
        ]
      }
    }
  }
}
'
fi
