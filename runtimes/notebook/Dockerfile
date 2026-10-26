# Single-user notebook image. Kernels talk to remote clusters through
# Spark Connect (sc://...) so the notebook pod itself stays small.
FROM quay.io/jupyter/minimal-notebook:python-3.11

RUN pip install --no-cache-dir \
      "pyspark[connect]==3.5.3" \
      "pyiceberg[s3fs,pyarrow]==0.8.1" \
      "mlflow==2.18.0" \
      "ray[client]==2.40.0" \
      "jupyter-collaboration==2.1.4" \
      "pandas==2.2.3"

COPY lp_magic.py /opt/lp/lp_magic.py
ENV PYTHONPATH=/opt/lp
# Auto-load the %lp magics in every IPython kernel.
RUN mkdir -p /home/jovyan/.ipython/profile_default/startup && \
    echo "import lp_magic; lp_magic.load()" > /home/jovyan/.ipython/profile_default/startup/00-lp.py
