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

output "external_ip_name" {
  description = "The external IPv4 name assigned to the global fowarding rule."
  value       = google_compute_global_address.default.name
}

output "external_ip_address" {
  description = "The external IPv4 assigned to the global fowarding rule."
  value       = google_compute_global_address.default.address
}

output "cloud_run_address" {
  description = "The uri assigned to the cloud run service. For testing before lb is set up."
  value = google_cloud_run_v2_service.metrics.uri
}

output "cloud_run_agent_email" {
  description = "Cloud run service agent email for CI/CD."
  value = google_project_service_identity.run_agent.email
}
