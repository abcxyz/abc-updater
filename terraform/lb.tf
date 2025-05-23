# Copyright 2023 The Authors (see AUTHORS file)
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

resource "random_id" "default" {
  byte_length = 2
}

resource "random_id" "cert" {
  keepers = {
    domains = join(",", var.domains)
  }

  byte_length = 2
}

resource "google_project_service" "services" {
  for_each = toset([
    "cloudresourcemanager.googleapis.com",
    "compute.googleapis.com",
    "run.googleapis.com",
    "serviceusage.googleapis.com",
  ])

  project = var.project_id

  service                    = each.value
  disable_on_destroy         = false
  disable_dependent_services = false
}

resource "google_compute_global_address" "default" {
  project = var.project_id

  name       = "${substr(var.name, 0, 50)}-${random_id.default.hex}-address" # 63 character limit
  ip_version = "IPV4"

  depends_on = [
    google_project_service.services["compute.googleapis.com"],
  ]
}

resource "google_compute_global_forwarding_rule" "http" {
  project = var.project_id

  name                  = "${substr(var.name, 0, 53)}-${random_id.default.hex}-http" # 63 character limit
  target                = google_compute_target_http_proxy.default.self_link
  ip_address            = google_compute_global_address.default.address
  port_range            = "80"
  load_balancing_scheme = "EXTERNAL"
}

resource "google_compute_global_forwarding_rule" "https" {
  project = var.project_id

  name                  = "${substr(var.name, 0, 52)}-${random_id.cert.hex}-https" # 63 character limit
  target                = google_compute_target_https_proxy.default.self_link
  ip_address            = google_compute_global_address.default.address
  port_range            = "443"
  load_balancing_scheme = "EXTERNAL"
}

resource "google_compute_managed_ssl_certificate" "default" {
  project = var.project_id

  name = "${substr(var.name, 0, 53)}-${random_id.cert.hex}-cert" # 63 character limit

  managed {
    domains = toset(var.domains)
  }

  depends_on = [
    google_project_service.services["compute.googleapis.com"],
  ]
  lifecycle {
    create_before_destroy = true
  }
}
resource "google_compute_url_map" "default" {
  project = var.project_id

  name            = "${substr(var.name, 0, 50)}-${random_id.default.hex}-url-map" # 63 character limit
  default_service = google_compute_backend_bucket.default.self_link

  host_rule {
    hosts        = [var.metrics_service_host]
    path_matcher = "metrics"
  }

  path_matcher {
    name            = "metrics"
    default_service = google_compute_backend_service.metrics_backend.id
  }
}

resource "google_compute_url_map" "https_redirect" {
  project = var.project_id

  name = "${substr(var.name, 0, 40)}-${random_id.default.hex}-https-redirect" # 63 character limit
  default_url_redirect {
    https_redirect         = true
    redirect_response_code = "MOVED_PERMANENTLY_DEFAULT"
    strip_query            = false
  }

  depends_on = [
    google_project_service.services["compute.googleapis.com"],
  ]
}

resource "google_compute_target_http_proxy" "default" {
  project = var.project_id

  name = "${substr(var.name, 0, 47)}-${random_id.default.hex}-http-proxy" # 63 character limit

  url_map = google_compute_url_map.https_redirect.self_link
}

resource "google_compute_target_https_proxy" "default" {
  project = var.project_id

  name    = "${substr(var.name, 0, 46)}-${random_id.cert.hex}-https-proxy" # 63 character limit
  url_map = google_compute_url_map.default.self_link

  ssl_certificates = [google_compute_managed_ssl_certificate.default.self_link]
}

resource "google_compute_backend_bucket" "default" {
  project = var.project_id

  name        = "${substr(var.name, 0, 44)}-${random_id.default.hex}-backend-bucket" # 63 character limit
  description = "${var.name} backend bucket"
  bucket_name = google_storage_bucket.default.name
  enable_cdn  = true

  depends_on = [
    google_project_service.services["compute.googleapis.com"],
  ]
}

resource "google_compute_region_network_endpoint_group" "metrics_neg" {
  project = var.project_id

  region                = var.compute_region
  name                  = "metrics-neg"
  network_endpoint_type = "SERVERLESS"

  cloud_run {
    service = google_cloud_run_v2_service.metrics.name
  }

  depends_on = [
    google_project_service.services["compute.googleapis.com"],
  ]

  lifecycle {
    create_before_destroy = true
  }
}

resource "google_compute_backend_service" "metrics_backend" {
  project = var.project_id

  name                  = "metrics-backend"
  load_balancing_scheme = "EXTERNAL"
  description           = "ABC Metrics backend"

  backend {
    description = "ABC updater metrics serverless backend group"
    group       = google_compute_region_network_endpoint_group.metrics_neg.id
  }

  log_config {
    enable = false
  }
}
