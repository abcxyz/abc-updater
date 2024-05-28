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
  description = "The name of the GCS bucket."
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

variable "metrics_service_location" {
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
