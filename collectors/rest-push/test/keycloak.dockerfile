# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
FROM quay.io/keycloak/keycloak:20.0
WORKDIR /opt/keycloak
COPY keycloak-realm.json data/import/keycloak-realm.json
ENV KC_HOSTNAME=localhost
ENV KEYCLOAK_USER=admin
ENV KEYCLOAK_PASSWORD=secret
ENV KEYCLOAK_ADMIN=admin
ENV KEYCLOAK_ADMIN_PASSWORD=secret
ENV KC_FEATURES=account-api,account2,authorization,client-policies,impersonation,docker,scripts,upload_scripts,admin-fine-grained-authz
# If you need to recreate the realm json file, you must export it via command line
# Attach to the docker container of your keycloak instance
# /opt/keycloak/bin/kc.sh export --file test-realm.json --users realm_file
# Then copy the exported file to the host:
# docker cp :/opt/keycloak/test-realm.json test/keycloak-realm.json
#RUN /opt/keycloak/bin/kc.sh import --file /opt/keycloak/data/import/keycloak-realm.json
ENTRYPOINT ["/opt/keycloak/bin/kc.sh"]
