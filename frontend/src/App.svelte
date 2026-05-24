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

<script>
    import { Router, Route } from "svelte-routing";
    import SearchBar from "./components/SearchBar.svelte";
    import StopPage from "./pages/StopPage.svelte";
    import RoutePage from "./pages/RoutePage.svelte";
    import BlockPage from "./pages/BlockPage.svelte";
    import VehiclePage from "./pages/VehiclePage.svelte";
    import TripPage from "./pages/TripPage.svelte";

    import logo from "./assets/logo.svg";

    export let url = "";
</script>

<h1 id="title"><img alt="logo" src={logo} height="50" /> HalteRadar</h1>

<SearchBar />

<Router {url}>
    <Route path="/stop/:stop" let:params>
        <StopPage stop={params.stop} />
    </Route>
    <Route path="/block/:data_owner/:block" let:params>
        <BlockPage data_owner={params.data_owner} block={params.block} />
    </Route>
    <Route path="/vehicle/:data_owner/:vehicle" let:params>
        <VehiclePage data_owner={params.data_owner} vehicle={params.vehicle} />
    </Route>
    <Route path="/route/:route" let:params>
        <RoutePage route={params.route} />
    </Route>
    <Route path="/trip/:trip" let:params>
        <TripPage trip={params.trip} />
    </Route>
    <Route path="/">
        <StopPage stop={"stoparea:449933"} />
    </Route>
    <Route>404 not found</Route>
</Router>

<footer class="footer">
    &copy; 2026 Friedel Schön &mdash; 
    Source hosted at <a href="https://github.com/friedelschoen/halteradar" target="_blank">
        GitHub
    </a>
</footer>
