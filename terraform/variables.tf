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

variable "project_id" {
  description = "The GCP project ID."
  type        = string
}

variable "bucket_name" {
  description = "The name of the GCS bucket holding update and metrics definitions."
  type        = string
}

variable "bucket_object_admins" {
  description = "IAM storage object admin members."
  type        = list(string)
}

variable "metrics_service_name" {
  description = "Name for Cloud Run service for metrics server."
  type        = string
}

variable "metrics_log_bucket_name" {
  description = "Name for Log Bucket metrics server logs are sent to. Must be unique."
  type        = string
}

variable "metrics_log_bucket_retention_days" {
  description = "Number of days to keep metrics logs."
  type        = number
  default     = 30
}

variable "metrics_log_bucket_viewers" {
  description = "IAM principals allowed to view metrics logs. Gives roles/logging.viewAccessor at project level."
  type        = list(string)
}

variable "compute_region" {
  description = "GCP Location to run Cloud Run service for metrics server."
  type        = string
}

variable "name" {
  type        = string
  description = "The name of this project."
  validation {
    condition     = can(regex("^[a-z][0-9a-z-]+[0-9a-z]$", var.name))
    error_message = "Name can only contain lowercase letters, numbers, hyphens(-) and must start with letter. Name will be truncated and suffixed with at random string if it exceeds requirements for a given resource."
  }
}

variable "domains" {
  type        = list(string)
  description = "Domain names to use for the HTTPS Global Load Balancer (e.g. [\"my-project.e2e.tycho.joonix.net\"])."
}

variable "metrics_envvars" {
  type        = map(string)
  default     = {}
  description = "Environment variables for the Metrics Cloud Run service (plain text)."
}

variable "ci_service_account_email" {
  type        = string
  description = "The service account email for deploying revisions to Cloud Run metrics server."
}

variable "metrics_service_host" {
  type = string
  description = "The host (foo.bar.com) domain traffice for metrics server will come on."
}
