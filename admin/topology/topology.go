package topology

import (
	"fmt"
	"strings"

	"TengShe/admin/printer"
	"TengShe/internal/messagelane"
	"TengShe/protocol"
	"TengShe/utils"
)

const (
	// Topology
	ADDNODE = iota
	GETUUID
	GETUUIDNUM
	CHECKNODE
	CALCULATE
	GETROUTE
	DELNODE
	REONLINENODE
	// User-friendly
	UPDATEDETAIL
	SHOWDETAIL
	SHOWTOPO
	UPDATEMEMO
	GETROUTEINFO
	GETALLROUTEINFO
)

// IDNum is only for user-friendly,uuid is used internally
type Topology struct {
	currentIDNum int
	nodes        map[int]*node        // we use uuidNum as the map's key,that's the only special exception
	route        map[string]RouteInfo // map[uuid]structured route
	history      map[string]int
	requests     *messagelane.Registry[*topoResult]

	TaskChan   chan *TopoTask
	ResultChan chan *topoResult
}

type RouteInfo struct {
	Target  string
	NextHop string
	Hops    []string
	Wire    string
}

type node struct {
	uuid            string
	parentUUID      string
	childrenUUID    []string
	currentUser     string
	currentHostname string
	currentIP       string
	memo            string
}

type TopoTask struct {
	Mode       int
	UUID       string
	UUIDNum    int
	ParentUUID string
	Target     *node
	HostName   string
	UserName   string
	Memo       string
	IsFirst    bool
	RequestID  messagelane.ID
}

type topoResult struct {
	IsExist   bool
	UUID      string
	Route     string
	RouteInfo RouteInfo
	IDNum     int
	AllNodes  []string
	AllRoutes []RouteInfo
}

func NewTopology() *Topology {
	topology := new(Topology)
	topology.nodes = make(map[int]*node)
	topology.route = make(map[string]RouteInfo)
	topology.history = make(map[string]int)
	topology.requests = messagelane.NewRegistry[*topoResult]()
	topology.currentIDNum = 0
	topology.TaskChan = make(chan *TopoTask)
	topology.ResultChan = make(chan *topoResult)
	return topology
}

func NewNode(uuid string, ip string) *node {
	node := new(node)
	node.uuid = uuid
	node.currentIP = ip
	return node
}

func (topology *Topology) Run() {
	for {
		task := <-topology.TaskChan
		switch task.Mode {
		case ADDNODE:
			topology.addNode(task)
		case GETUUID:
			topology.getUUID(task)
		case GETUUIDNUM:
			topology.getUUIDNum(task)
		case CHECKNODE:
			topology.checkNode(task)
		case UPDATEDETAIL:
			topology.updateDetail(task)
		case SHOWDETAIL:
			topology.showDetailWithTask(task)
		case SHOWTOPO:
			topology.showTopoWithTask(task)
		case UPDATEMEMO:
			topology.updateMemo(task)
		case CALCULATE:
			topology.calculateWithTask(task)
		case GETROUTE:
			topology.getRoute(task)
		case GETROUTEINFO:
			topology.getRouteInfo(task)
		case GETALLROUTEINFO:
			topology.getAllRouteInfo(task)
		case DELNODE:
			topology.delNode(task)
		case REONLINENODE:
			topology.reonlineNode(task)
		}
	}
}

func (topology *Topology) QueryUUID(uuidNum int) string {
	result := topology.request(&TopoTask{Mode: GETUUID, UUIDNum: uuidNum})
	return result.UUID
}

func (topology *Topology) QueryUUIDNum(uuid string) int {
	result := topology.request(&TopoTask{Mode: GETUUIDNUM, UUID: uuid})
	return result.IDNum
}

func (topology *Topology) QueryRoute(uuid string) string {
	return topology.QueryRouteInfo(uuid).Wire
}

func (topology *Topology) QueryRouteInfo(uuid string) RouteInfo {
	result := topology.request(&TopoTask{Mode: GETROUTEINFO, UUID: uuid})
	return result.RouteInfo
}

func (topology *Topology) QueryAllRouteInfo() []RouteInfo {
	result := topology.request(&TopoTask{Mode: GETALLROUTEINFO})
	return result.AllRoutes
}

func (topology *Topology) NodeExists(uuidNum int) bool {
	result := topology.request(&TopoTask{Mode: CHECKNODE, UUIDNum: uuidNum})
	return result.IsExist
}

func (topology *Topology) ShowDetail() {
	topology.request(&TopoTask{Mode: SHOWDETAIL})
}

func (topology *Topology) ShowTopo() {
	topology.request(&TopoTask{Mode: SHOWTOPO})
}

func (topology *Topology) Recalculate() {
	topology.request(&TopoTask{Mode: CALCULATE})
}

func (topology *Topology) AddNode(target *node, parentUUID string, isFirst bool) (bool, int) {
	result := topology.request(&TopoTask{
		Mode:       ADDNODE,
		Target:     target,
		ParentUUID: parentUUID,
		IsFirst:    isFirst,
	})
	return result.IsExist, result.IDNum
}

func (topology *Topology) DeleteNode(uuid string) []string {
	result := topology.request(&TopoTask{Mode: DELNODE, UUID: uuid})
	return result.AllNodes
}

func (topology *Topology) ReonlineNode(target *node, parentUUID string, isFirst bool) (bool, int) {
	result := topology.request(&TopoTask{
		Mode:       REONLINENODE,
		Target:     target,
		ParentUUID: parentUUID,
		IsFirst:    isFirst,
	})
	return result.IsExist, result.IDNum
}

func (topology *Topology) request(task *TopoTask) *topoResult {
	requestID, response, cancel := topology.requests.Open()
	defer cancel()

	task.RequestID = requestID
	topology.TaskChan <- task

	result, ok := <-response
	if !ok {
		return &topoResult{}
	}
	return result
}

func (topology *Topology) reply(task *TopoTask, result *topoResult) {
	if task != nil && task.RequestID != messagelane.NoID {
		topology.requests.Resolve(task.RequestID, result)
		return
	}
	topology.ResultChan <- result
}

func (topology *Topology) id2IDNum(uuid string) int {
	for idNum, tNode := range topology.nodes {
		if tNode.uuid == uuid {
			return idNum
		}
	}
	return -1
}

func (topology *Topology) idNum2ID(uuidNum int) string {
	return topology.nodes[uuidNum].uuid
}

func (topology *Topology) getUUID(task *TopoTask) {
	topology.reply(task, &topoResult{UUID: topology.idNum2ID(task.UUIDNum)})
}

func (topology *Topology) getUUIDNum(task *TopoTask) {
	topology.reply(task, &topoResult{IDNum: topology.id2IDNum(task.UUID)})
}

func (topology *Topology) checkNode(task *TopoTask) {
	if _, ok := topology.nodes[task.UUIDNum]; ok {
		topology.reply(task, &topoResult{IsExist: true})
	} else {
		topology.reply(task, &topoResult{IsExist: false})
	}
}

func (topology *Topology) addNode(task *TopoTask) {
	if task.Target == nil {
		topology.reply(task, &topoResult{IsExist: false, IDNum: -1})
		return
	}

	if idNum := topology.id2IDNum(task.Target.uuid); idNum >= 0 {
		topology.reply(task, &topoResult{IsExist: true, IDNum: idNum})
		return
	}

	if task.IsFirst {
		task.Target.parentUUID = protocol.ADMIN_UUID
	} else {
		task.Target.parentUUID = task.ParentUUID
		parentIDNum := topology.id2IDNum(task.ParentUUID)
		if parentIDNum >= 0 {
			topology.appendChild(parentIDNum, task.Target.uuid)
		} else {
			topology.reply(task, &topoResult{IsExist: false, IDNum: -1})
			return
		}
	}

	topology.nodes[topology.currentIDNum] = task.Target

	topology.history[task.Target.uuid] = topology.currentIDNum

	topology.reply(task, &topoResult{IDNum: topology.currentIDNum})

	topology.currentIDNum++
}

func (topology *Topology) calculate() {
	topology.calculateWithTask(nil)
}

func (topology *Topology) calculateWithTask(task *TopoTask) {
	newRouteInfo := make(map[string]RouteInfo) // Create a new route map

	for currentIDNum := range topology.nodes {
		currentID := topology.nodes[currentIDNum].uuid
		newRouteInfo[currentID] = calculateRouteInfo(topology.nodes, currentIDNum)
	}

	topology.route = newRouteInfo

	topology.reply(task, &topoResult{}) // Just tell upstream: work done!
}

func calculateRoute(nodes map[int]*node, currentIDNum int) string {
	return calculateRouteInfo(nodes, currentIDNum).Wire
}

func calculateRouteInfo(nodes map[int]*node, currentIDNum int) RouteInfo {
	var tempRoute []string
	tempIDNum := currentIDNum
	target := nodes[currentIDNum].uuid

	if nodes[currentIDNum].parentUUID == protocol.ADMIN_UUID {
		return RouteInfo{Target: target}
	}

	for {
		if nodes[tempIDNum].parentUUID != protocol.ADMIN_UUID {
			tempRoute = append(tempRoute, nodes[tempIDNum].uuid)
			for nextIDNum := range nodes { // Bug fix: thanks to @lz520520
				if nodes[nextIDNum].uuid == nodes[tempIDNum].parentUUID {
					tempIDNum = nextIDNum
					break
				}
			}
		} else {
			utils.StringSliceReverse(tempRoute)
			hops := append([]string(nil), tempRoute...)
			route := RouteInfo{
				Target: target,
				Hops:   hops,
				Wire:   strings.Join(hops, ":"),
			}
			if len(hops) > 0 {
				route.NextHop = hops[0]
			}
			return route
		}
	}
}

func (topology *Topology) getRoute(task *TopoTask) {
	route := topology.routeInfo(task.UUID)
	topology.reply(task, &topoResult{Route: route.Wire, RouteInfo: route})
}

func (topology *Topology) getRouteInfo(task *TopoTask) {
	route := topology.routeInfo(task.UUID)
	topology.reply(task, &topoResult{Route: route.Wire, RouteInfo: route})
}

func (topology *Topology) getAllRouteInfo(task *TopoTask) {
	routes := make([]RouteInfo, 0, len(topology.route))
	for uuid := range topology.route {
		routes = append(routes, topology.routeInfo(uuid))
	}
	topology.reply(task, &topoResult{AllRoutes: routes})
}

func (topology *Topology) routeInfo(uuid string) RouteInfo {
	route := topology.route[uuid]
	route.Hops = append([]string(nil), route.Hops...)
	return route
}

func (topology *Topology) updateDetail(task *TopoTask) {
	uuidNum := topology.id2IDNum(task.UUID)
	if uuidNum >= 0 {
		topology.nodes[uuidNum].currentUser = task.UserName
		topology.nodes[uuidNum].currentHostname = task.HostName
		topology.nodes[uuidNum].memo = task.Memo
	}
}

func (topology *Topology) showDetail() {
	topology.showDetailWithTask(nil)
}

func (topology *Topology) showDetailWithTask(task *TopoTask) {
	var nodes []int
	for uuidNum := range topology.nodes {
		nodes = append(nodes, uuidNum)
	}

	utils.CheckRange(nodes)

	for _, uuidNum := range nodes {
		fmt.Printf("\r\nNode[%d] -> IP: %s  Hostname: %s  User: %s\r\nMemo: %s\r\n",
			uuidNum,
			topology.nodes[uuidNum].currentIP,
			topology.nodes[uuidNum].currentHostname,
			topology.nodes[uuidNum].currentUser,
			topology.nodes[uuidNum].memo,
		)
	}

	topology.reply(task, &topoResult{}) // Just tell upstream: work done!
}

func (topology *Topology) showTopo() {
	topology.showTopoWithTask(nil)
}

func (topology *Topology) showTopoWithTask(task *TopoTask) {
	fmt.Print("\r\n[admin]\r\n")

	visited := make(map[int]bool, len(topology.nodes))
	roots := topology.rootNodeIDs()
	for i, uuidNum := range roots {
		topology.printTopoNode(uuidNum, "", i == len(roots)-1, visited)
	}

	detached := topology.unvisitedNodeIDs(visited)
	if len(detached) > 0 {
		fmt.Print("[detached]\r\n")
		for i, uuidNum := range detached {
			topology.printTopoNode(uuidNum, "", i == len(detached)-1, visited)
		}
	}

	topology.reply(task, &topoResult{}) // Just tell upstream: work done!
}

func (topology *Topology) rootNodeIDs() []int {
	var roots []int
	for uuidNum, target := range topology.nodes {
		if target == nil {
			continue
		}
		if target.parentUUID == protocol.ADMIN_UUID || topology.id2IDNum(target.parentUUID) < 0 {
			roots = append(roots, uuidNum)
		}
	}
	utils.CheckRange(roots)
	return roots
}

func (topology *Topology) childNodeIDs(target *node) []int {
	var children []int
	if target == nil {
		return children
	}
	for _, childUUID := range target.childrenUUID {
		childIDNum := topology.id2IDNum(childUUID)
		if childIDNum >= 0 {
			children = append(children, childIDNum)
		}
	}
	utils.CheckRange(children)
	return children
}

func (topology *Topology) unvisitedNodeIDs(visited map[int]bool) []int {
	var nodes []int
	for uuidNum := range topology.nodes {
		if !visited[uuidNum] {
			nodes = append(nodes, uuidNum)
		}
	}
	utils.CheckRange(nodes)
	return nodes
}

func (topology *Topology) printTopoNode(uuidNum int, prefix string, isLast bool, visited map[int]bool) {
	connector := "|-- "
	nextPrefix := prefix + "|   "
	if isLast {
		connector = "`-- "
		nextPrefix = prefix + "    "
	}

	target, ok := topology.nodes[uuidNum]
	if !ok || target == nil {
		fmt.Printf("%s%sNode[%d]  missing\r\n", prefix, connector, uuidNum)
		return
	}

	if visited[uuidNum] {
		fmt.Printf("%s%sNode[%d]  already shown\r\n", prefix, connector, uuidNum)
		return
	}
	visited[uuidNum] = true

	fmt.Printf("%s%s%s\r\n", prefix, connector, formatTopoNode(uuidNum, target))

	children := topology.childNodeIDs(target)
	for i, childIDNum := range children {
		topology.printTopoNode(childIDNum, nextPrefix, i == len(children)-1, visited)
	}
}

func formatTopoNode(uuidNum int, target *node) string {
	parts := []string{fmt.Sprintf("Node[%d]", uuidNum)}
	if currentIP := compactTopoField(target.currentIP); currentIP != "" {
		parts = append(parts, "ip="+currentIP)
	}

	identity := strings.Trim(compactTopoField(target.currentUser)+"@"+compactTopoField(target.currentHostname), "@")
	if identity != "" {
		parts = append(parts, identity)
	}

	if memo := compactTopoField(target.memo); memo != "" {
		parts = append(parts, "memo="+memo)
	}
	return strings.Join(parts, "  ")
}

func compactTopoField(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func (topology *Topology) updateMemo(task *TopoTask) {
	uuidNum := topology.id2IDNum(task.UUID)
	if uuidNum >= 0 {
		topology.nodes[uuidNum].memo = task.Memo
	}
}

func (topology *Topology) delNode(task *TopoTask) {
	// find all children node,del them
	var ready []int
	var readyUUID []string

	idNum := topology.id2IDNum(task.UUID)
	if idNum < 0 {
		topology.reply(task, &topoResult{})
		return
	}

	parentIDNum := topology.id2IDNum(topology.nodes[idNum].parentUUID)
	if parentIDNum >= 0 {
		for pointer, childUUID := range topology.nodes[parentIDNum].childrenUUID { // del parent's children record
			if childUUID == task.UUID {
				if pointer == len(topology.nodes[parentIDNum].childrenUUID)-1 {
					topology.nodes[parentIDNum].childrenUUID = topology.nodes[parentIDNum].childrenUUID[:pointer]
				} else {
					topology.nodes[parentIDNum].childrenUUID = append(topology.nodes[parentIDNum].childrenUUID[:pointer], topology.nodes[parentIDNum].childrenUUID[pointer+1:]...)
				}
			}
		}
	}

	topology.findChildrenNodes(&ready, idNum)

	ready = append(ready, idNum)

	for _, idNum := range ready {
		printer.Fail("\r\n[*] Node %d is offline!", idNum)
		readyUUID = append(readyUUID, topology.idNum2ID(idNum))
		delete(topology.route, topology.idNum2ID(idNum))
		delete(topology.nodes, idNum)
	}

	topology.reply(task, &topoResult{AllNodes: readyUUID})
}

func (topology *Topology) findChildrenNodes(ready *[]int, idNum int) {
	for _, uuid := range topology.nodes[idNum].childrenUUID {
		idNum := topology.id2IDNum(uuid)
		*ready = append(*ready, idNum)
		topology.findChildrenNodes(ready, idNum)
	}
}

func (topology *Topology) reonlineNode(task *TopoTask) {
	if task.Target == nil {
		topology.reply(task, &topoResult{IsExist: false, IDNum: -1})
		return
	}

	if task.IsFirst {
		task.Target.parentUUID = protocol.ADMIN_UUID
	} else {
		task.Target.parentUUID = task.ParentUUID
		parentIDNum := topology.id2IDNum(task.ParentUUID)
		if parentIDNum < 0 {
			topology.reply(task, &topoResult{IsExist: false, IDNum: -1})
			return
		}
		topology.appendChild(parentIDNum, task.Target.uuid)
	}

	var idNum int
	if _, ok := topology.history[task.Target.uuid]; ok {
		idNum = topology.history[task.Target.uuid]
	} else {
		idNum = topology.currentIDNum
		topology.history[task.Target.uuid] = idNum
		topology.currentIDNum++
	}

	topology.nodes[idNum] = task.Target

	topology.reply(task, &topoResult{})
}

func (topology *Topology) appendChild(parentIDNum int, childUUID string) {
	for _, uuid := range topology.nodes[parentIDNum].childrenUUID {
		if uuid == childUUID {
			return
		}
	}

	topology.nodes[parentIDNum].childrenUUID = append(topology.nodes[parentIDNum].childrenUUID, childUUID)
}
