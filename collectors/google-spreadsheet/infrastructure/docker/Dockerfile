FROM eclipse-temurin:17-jre-alpine
WORKDIR /app
COPY target/app.jar app.jar
# Needs some GC tuning because we have temporary memory spikes at every sync that aren't cleared at default configuration
ENTRYPOINT [ "sh", "-c", "java $JAVA_OPTS -XX:G1PeriodicGCSystemLoadThreshold=0 -XX:G1PeriodicGCInterval=20000 -XX:-G1PeriodicGCInvokesConcurrent -XX:+UseG1GC -jar app.jar"]

