# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
COPY target/app.jar app.jar
ENTRYPOINT [ "java", "-jar", "app.jar"]

