# LP Spark runtime: Spark 3.5 + Spark Connect + Iceberg REST/S3 + MLflow.
# Version it like a product ("lp-1.0-spark3.5") and pin everything.
ARG SPARK_VERSION=3.5.3
FROM apache/spark:${SPARK_VERSION}-scala2.12-java17-python3-ubuntu

ARG SPARK_VERSION=3.5.3
ARG ICEBERG_VERSION=1.7.1
ARG SCALA=2.12

USER root
RUN set -eux; cd /opt/spark/jars; \
    base=https://repo1.maven.org/maven2; \
    curl -fsSLO $base/org/apache/iceberg/iceberg-spark-runtime-3.5_${SCALA}/${ICEBERG_VERSION}/iceberg-spark-runtime-3.5_${SCALA}-${ICEBERG_VERSION}.jar; \
    curl -fsSLO $base/org/apache/iceberg/iceberg-aws-bundle/${ICEBERG_VERSION}/iceberg-aws-bundle-${ICEBERG_VERSION}.jar; \
    curl -fsSLO $base/org/apache/spark/spark-connect_${SCALA}/${SPARK_VERSION}/spark-connect_${SCALA}-${SPARK_VERSION}.jar

RUN pip install --no-cache-dir "mlflow-skinny==2.18.0" "pyarrow==17.0.0" "pandas==2.2.3"

# Phase 2: add Kyuubi Spark AuthZ (Ranger) plugin jar here.
USER 185
