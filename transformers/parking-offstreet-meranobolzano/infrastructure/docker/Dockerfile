# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
COPY target/app.jar app.jar
ENTRYPOINT [ "sh", "-c", "java $JAVA_OPTS -XX:G1PeriodicGCSystemLoadThreshold=0 -XX:G1PeriodicGCInterval=20000 -XX:-G1PeriodicGCInvokesConcurrent -XX:+UseG1GC -jar app.jar"]

