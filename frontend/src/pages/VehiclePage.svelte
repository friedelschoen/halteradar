<!--
Copyright (C) 2026 Friedel Schön

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <http://www.gnu.org/licenses/>.
-->

<script lang="ts">
    import TripTablePage from "./TripTablePage.svelte";

    export let data_owner: string;
    export let vehicle: string;

    $: endpoint =
        "/api/vehicle/" +
        encodeURIComponent(data_owner) +
        "/" +
        encodeURIComponent(vehicle);

    $: title = "Vehicle " + vehicle;
</script>

<TripTablePage
    {title}
    {endpoint}
    emptyText="No trips found for this vehicle."
/>
