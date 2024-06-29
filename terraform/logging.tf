# Copyright 2024 The Authors (see AUTHORS file)
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

resource "google_logging_project_bucket_config" "metrics" {
  project = var.project_id

  location         = var.compute_region
  retention_days   = var.metrics_log_bucket_retention_days
  bucket_id        = var.metrics_log_bucket_name
  enable_analytics = true
}

resource "google_logging_project_sink" "metrics" {
  project = var.project_id

  name                   = "abc-updater-metrics-sink"
  destination            = "logging.googleapis.com/${google_logging_project_bucket_config.metrics.name}"
  unique_writer_identity = true
  filter = <<EOF
  resource.type="cloud_run_revision" AND
  resource.labels.service_name="${var.metrics_service_name}" AND
  jsonPayload.message="metric received"
  EOF

}


resource "google_logging_log_view_iam_member" "metric-log" {
  for_each = toset(var.metrics_log_bucket_viewers)

  parent   = var.project_id
  location = var.compute_region
  bucket   = var.metrics_log_bucket_name
  name     = "_AllLogs"
  role     = "roles/logging.viewer"
  member   = each.key
}

resource "google_logging_log_view_iam_member" "metric-log-view" {
  for_each = toset(var.metrics_log_bucket_viewers)

  parent   = var.project_id
  location = var.compute_region
  bucket   = var.metrics_log_bucket_name
  name     = "_AllLogs"
  role     = "roles/logging.viewAccessor"
  member   = each.key
}

#resource "google_project_iam_member" "metric_viewers" {
#  for_each = toset(var.metrics_log_bucket_viewers)
#
#  project = var.project_id
#
#  role   = "roles/logging.viewAccessor"
#  member = each.key
#
#  condition {
#    expression = "resource.name == \"projects/${var.project_id}/\""
#    title      = "Only Metrics Bucket View"
#  }
#}
#
## WIP: rapid prototyping. Will go inside of module.
#resource "google_project_iam_member" "metric_viewers" {
#  for_each = toset(var.metrics_log_bucket_viewers)
#
#  project = var.project_id
#
#  role   = google_project_iam_custom_role.metric_viewers.id
#  member = "group:chp-bets-platform-dev@twosync.google.com"
#}
#
#resource "google_project_iam_custom_role" "metric_viewers" {
#  project = var.project_id
#
#  role_id     = "metricViewers"
#  title       = "Metric Views"
#  description = "Minimal permissions to view metrics logs."
#  permissions = [
#    "logging.buckets.list",
#    "logging.buckets.get",
#  ]
#}
