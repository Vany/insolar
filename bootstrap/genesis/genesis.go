//
// Copyright 2019 Insolar Technologies GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package genesis

import (
	"context"
	"crypto"
	"encoding/json"
	"fmt"
	"github.com/insolar/insolar/application/contract/mdcenter"
	"io/ioutil"
	"path"
	"path/filepath"
	"strconv"

	"github.com/insolar/insolar/application/contract/member"
	"github.com/insolar/insolar/application/contract/nodedomain"
	"github.com/insolar/insolar/application/contract/noderecord"
	"github.com/insolar/insolar/application/contract/rootdomain"
	"github.com/insolar/insolar/application/contract/wallet"
	"github.com/insolar/insolar/certificate"
	"github.com/insolar/insolar/insolar"
	"github.com/insolar/insolar/insolar/message"
	"github.com/insolar/insolar/instrumentation/inslogger"
	"github.com/insolar/insolar/internal/ledger/artifact"
	"github.com/insolar/insolar/platformpolicy"
	"github.com/pkg/errors"
)

const (
	nodeDomain       = "nodedomain"
	nodeRecord       = "noderecord"
	rootDomain       = "rootdomain"
	walletContract   = "wallet"
	memberContract   = "member"
	mdcenterContract = "mdcenter"
)

var contractNames = []string{walletContract, memberContract, rootDomain, nodeDomain, nodeRecord, mdcenterContract}

type nodeInfo struct {
	privateKey crypto.PrivateKey
	publicKey  string
}

// Generator is a component for generating RootDomain instance and genesis contracts.
type Generator struct {
	artifactManager artifact.Manager
	config          *Config

	genesisRef       insolar.Reference
	rootDomainRef    *insolar.Reference
	nodeDomainRef    *insolar.Reference
	rootMemberRef    *insolar.Reference
	mdAdminMemberRef *insolar.Reference
	mdCenterRef      *insolar.Reference

	keyOut string
}

// NewGenerator creates new Generator.
func NewGenerator(config *Config, am artifact.Manager, genesisRef insolar.Reference, genesisKeyOut string) *Generator {
	return &Generator{
		artifactManager: am,
		config:          config,

		genesisRef:    genesisRef,
		rootDomainRef: &insolar.Reference{},

		keyOut: genesisKeyOut,
	}
}

// Run generates genesis data.
func (g *Generator) Run(ctx context.Context) error {
	inslog := inslogger.FromContext(ctx)
	inslog.Info("[ Genesis ] Starting  ...")
	defer inslog.Info("[ Genesis ] Finished.")

	rootDomainID, err := g.artifactManager.RegisterRequest(
		ctx,
		g.genesisRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{
				Name: rootDomain,
			},
		},
	)
	if err != nil {
		panic(errors.Wrap(err, "[ Genesis ] Couldn't create rootdomain instance"))
	}

	inslog.Info("[ Genesis ] newContractBuilder ...")
	cb := newContractBuilder(g.genesisRef, g.artifactManager)
	defer cb.clean()

	inslog.Info("[ Genesis ] buildSmartContracts ...")
	prototypes, err := cb.buildPrototypes(ctx, rootDomainID)
	if err != nil {
		panic(errors.Wrap(err, "[ Genesis ] couldn't build contracts"))
	}

	inslog.Info("[ Genesis ] getKeysFromFile ...")
	_, rootPubKey, err := getKeysFromFile(ctx, g.config.RootKeysFile)
	if err != nil {
		return errors.Wrap(err, "[ Genesis ] couldn't get root keys")
	}

	inslog.Info("[ Genesis ] getKeysFromFile for mdAdmin ...")
	_, mdAdminPubKey, err := getKeysFromFile(ctx, g.config.MDAdminKeysFile)
	if err != nil {
		return errors.Wrap(err, "[ Genesis ] couldn't get root keys")
	}

	inslog.Info("[ Genesis ] getKeysFromFile for oracles ...")
	oracleMap := map[string]string{}
	for _, o := range g.config.OracleKeysFiles {
		_, oraclePubKey, err := getKeysFromFile(ctx, o.KeysFile)
		if err != nil {
			return errors.Wrap(err, "[ Genesis ] couldn't get oracle keys for oracle: "+o.Name)
		}
		oracleMap[o.Name] = oraclePubKey
	}

	inslog.Info("[ Genesis ] activateSmartContracts ...")
	nodes, err := g.activateSmartContracts(ctx, cb, rootPubKey, mdAdminPubKey, oracleMap, rootDomainID, prototypes)
	if err != nil {
		panic(errors.Wrap(err, "[ Genesis ] could't activate smart contracts"))
	}

	inslog.Info("[ Genesis ] makeCertificates ...")
	err = g.makeCertificates(nodes)
	if err != nil {
		return errors.Wrap(err, "[ Genesis ] Couldn't generate discovery certificates")
	}

	return nil
}

func (g *Generator) activateRootDomain(
	ctx context.Context,
	cb *contractsBuilder,
	contractID *insolar.ID,
) (artifact.ObjectDescriptor, error) {
	rd, err := rootdomain.NewRootDomain()
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateRootDomain ]")
	}

	instanceData, err := insolar.Serialize(rd)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateRootDomain ]")
	}

	contract := insolar.NewReference(*contractID, *contractID)
	desc, err := g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		g.genesisRef,
		*cb.prototypes[rootDomain],
		false,
		instanceData,
	)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateRootDomain ] Couldn't create rootdomain instance")
	}
	_, err = g.artifactManager.RegisterResult(ctx, g.genesisRef, *contract, nil)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateRootDomain ] Couldn't create rootdomain instance")
	}
	g.rootDomainRef = contract

	return desc, nil
}

func (g *Generator) activateNodeDomain(
	ctx context.Context, domain *insolar.ID, nodeDomainProto insolar.Reference,
) (artifact.ObjectDescriptor, error) {
	nd, _ := nodedomain.NewNodeDomain()

	instanceData, err := insolar.Serialize(nd)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateNodeDomain ] node domain serialization")
	}

	contractID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: "NodeDomain"},
		},
	)

	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateNodeDomain ] couldn't create nodedomain instance")
	}
	contract := insolar.NewReference(*domain, *contractID)
	desc, err := g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.rootDomainRef,
		nodeDomainProto,
		false,
		instanceData,
	)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateNodeDomain ] couldn't create nodedomain instance")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return nil, errors.Wrap(err, "[ ActivateNodeDomain ] couldn't create nodedomain instance")
	}

	g.nodeDomainRef = contract

	return desc, nil
}

func (g *Generator) activateRootMember(
	ctx context.Context,
	domain *insolar.ID,
	rootPubKey string,
	memberContractProto insolar.Reference,
) error {

	m, err := member.New("RootMember", rootPubKey)
	if err != nil {
		return errors.Wrap(err, "[ ActivateRootMember ]")
	}

	instanceData, err := insolar.Serialize(m)
	if err != nil {
		return errors.Wrap(err, "[ ActivateRootMember ]")
	}

	contractID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: "RootMember"},
		},
	)

	if err != nil {
		return errors.Wrap(err, "[ ActivateRootMember ] couldn't create root member instance")
	}
	contract := insolar.NewReference(*domain, *contractID)
	_, err = g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.rootDomainRef,
		memberContractProto,
		false,
		instanceData,
	)
	if err != nil {
		return errors.Wrap(err, "[ ActivateRootMember ] couldn't create root member instance")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return errors.Wrap(err, "[ ActivateRootMember ] couldn't create root member instance")
	}
	g.rootMemberRef = contract
	return nil
}

func (g *Generator) activateMDAdminMember(
	ctx context.Context,
	domain *insolar.ID,
	mdAdminPubKey string,
	memberContractProto insolar.Reference,
) error {

	m, err := member.New("MDAdminMember", mdAdminPubKey)
	if err != nil {
		return errors.Wrap(err, "[ activateMDAdminMember ]")
	}

	instanceData, err := insolar.Serialize(m)
	if err != nil {
		return errors.Wrap(err, "[ activateMDAdminMember ]")
	}

	contractID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: "MDAdminMember"},
		},
	)

	if err != nil {
		return errors.Wrap(err, "[ activateMDAdminMember ] couldn't create mdAdmin member instance")
	}
	contract := insolar.NewReference(*domain, *contractID)
	_, err = g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.rootDomainRef,
		memberContractProto,
		false,
		instanceData,
	)
	if err != nil {
		return errors.Wrap(err, "[ activateMDAdminMember ] couldn't create mdAdmin member instance")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return errors.Wrap(err, "[ activateMDAdminMember ] couldn't create mdAdmin member instance")
	}
	g.mdAdminMemberRef = contract
	return nil
}

func (g *Generator) activateMDCenter(
	ctx context.Context,
	domain *insolar.ID,
	oraclePubKeys map[string]string,
	memberContractProto insolar.Reference,
) error {

	mdc, err := mdcenter.New(oraclePubKeys)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenter ]")
	}

	instanceData, err := insolar.Serialize(mdc)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenter ]")
	}

	contractID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: "MDCenter"},
		},
	)

	if err != nil {
		return errors.Wrap(err, "[ activateMDCenter ] couldn't create MDCenter instance")
	}
	contract := insolar.NewReference(*domain, *contractID)
	_, err = g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.rootDomainRef,
		memberContractProto,
		true,
		instanceData,
	)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenter ] couldn't create MDCenter instance")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenter ] couldn't create MDCenter instance")
	}
	g.mdCenterRef = contract
	return nil
}

func (g *Generator) activateOracleMembers(
	ctx context.Context,
	domain *insolar.ID,
	oraclePubKeys map[string]string,
	mdcenterContractProto insolar.Reference,
) error {

	for name, key := range oraclePubKeys {
		o, err := member.New(name, key)
		if err != nil {
			return errors.Wrap(err, "[ activateOracleMembers ]")
		}

		instanceData, err := insolar.Serialize(o)
		if err != nil {
			return errors.Wrap(err, "[ activateOracleMembers ]")
		}

		contractID, err := g.artifactManager.RegisterRequest(
			ctx,
			*g.rootDomainRef,
			&message.Parcel{
				Msg: &message.GenesisRequest{Name: name},
			},
		)

		if err != nil {
			return errors.Wrap(err, "[ activateOracleMembers ] couldn't create oracle member instance with name: "+name)
		}
		contract := insolar.NewReference(*domain, *contractID)
		_, err = g.artifactManager.ActivateObject(
			ctx,
			insolar.Reference{},
			*contract,
			*g.rootDomainRef,
			mdcenterContractProto,
			false,
			instanceData,
		)
		if err != nil {
			return errors.Wrap(err, "[ activateOracleMembers ] couldn't create oracle member instance with name: "+name)
		}
		_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
		if err != nil {
			return errors.Wrap(err, "[ activateOracleMembers ] couldn't create oracle member instance with name: "+name)
		}

	}
	return nil
}

// TODO: this is not required since we refer by request id.
func (g *Generator) updateRootDomain(
	ctx context.Context, domainDesc artifact.ObjectDescriptor,
) error {
	updateData, err := insolar.Serialize(&rootdomain.RootDomain{RootMember: *g.rootMemberRef, MDAdminMember: *g.mdAdminMemberRef, NodeDomainRef: *g.nodeDomainRef})
	if err != nil {
		return errors.Wrap(err, "[ updateRootDomain ]")
	}
	_, err = g.artifactManager.UpdateObject(
		ctx,
		insolar.Reference{},
		insolar.Reference{},
		domainDesc,
		updateData,
	)
	if err != nil {
		return errors.Wrap(err, "[ updateRootDomain ]")
	}

	return nil
}

func (g *Generator) activateMDCenterWallet(
	ctx context.Context, domain *insolar.ID, walletContractProto insolar.Reference,
) error {

	w, err := wallet.New(g.config.MDCenterBalance)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenterWallet ]")
	}

	instanceData, err := insolar.Serialize(w)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenterWallet ]")
	}

	contractID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: "MDCenterWallet"},
		},
	)

	if err != nil {
		return errors.Wrap(err, "[ activateMDCenterWallet ] couldn't create mdCenter wallet")
	}
	contract := insolar.NewReference(*domain, *contractID)
	_, err = g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.mdCenterRef,
		walletContractProto,
		true,
		instanceData,
	)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenterWallet ] couldn't create mdCenter wallet")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return errors.Wrap(err, "[ activateMDCenterWallet ] couldn't create mdCenter wallet")
	}

	return nil
}

func (g *Generator) activateSmartContracts(
	ctx context.Context,
	cb *contractsBuilder,
	rootPubKey string,
	mdAdminPubKey string,
	oracleMap map[string]string,
	rootDomainID *insolar.ID,
	prototypes prototypes,
) ([]genesisNode, error) {

	rootDomainDesc, err := g.activateRootDomain(ctx, cb, rootDomainID)
	errMsg := "[ ActivateSmartContracts ]"
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	nodeDomainDesc, err := g.activateNodeDomain(ctx, rootDomainID, *prototypes[nodeDomain])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	err = g.activateRootMember(ctx, rootDomainID, rootPubKey, *cb.prototypes[memberContract])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	err = g.activateMDAdminMember(ctx, rootDomainID, mdAdminPubKey, *cb.prototypes[memberContract])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	err = g.activateOracleMembers(ctx, rootDomainID, oracleMap, *cb.prototypes[memberContract])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	err = g.activateMDCenter(ctx, rootDomainID, oracleMap, *cb.prototypes[mdcenterContract])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}
	// TODO: this is not required since we refer by request id.
	err = g.updateRootDomain(ctx, rootDomainDesc)
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	indexMap := make(map[string]string)
	discoveryNodes, indexMap, err := g.addDiscoveryIndex(ctx, indexMap, *cb.prototypes[nodeRecord])
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	err = g.updateNodeDomainIndex(ctx, nodeDomainDesc, indexMap)
	if err != nil {
		return nil, errors.Wrap(err, errMsg)
	}

	return discoveryNodes, nil
}

type genesisNode struct {
	node    certificate.BootstrapNode
	privKey crypto.PrivateKey
	ref     *insolar.Reference
	role    string
}

func (g *Generator) activateDiscoveryNodes(
	ctx context.Context,
	nodeRecordProto insolar.Reference,
	nodesInfo []nodeInfo,
) ([]genesisNode, error) {
	if len(nodesInfo) != len(g.config.DiscoveryNodes) {
		return nil, errors.New("[ activateDiscoveryNodes ] len of nodesInfo param must be equal to len of DiscoveryNodes in genesis config")
	}

	nodes := make([]genesisNode, len(g.config.DiscoveryNodes))

	for i, discoverNode := range g.config.DiscoveryNodes {
		privKey := nodesInfo[i].privateKey
		nodePubKey := nodesInfo[i].publicKey

		nodeState := &noderecord.NodeRecord{
			Record: noderecord.RecordInfo{
				PublicKey: nodePubKey,
				Role:      insolar.GetStaticRoleFromString(discoverNode.Role),
			},
		}
		contract, err := g.activateNodeRecord(ctx, nodeState, "discoverynoderecord_"+strconv.Itoa(i), nodeRecordProto)
		if err != nil {
			return nil, errors.Wrap(err, "[ activateDiscoveryNodes ] Couldn't activateNodeRecord node instance")
		}

		nodes[i] = genesisNode{
			node: certificate.BootstrapNode{
				PublicKey: nodePubKey,
				Host:      discoverNode.Host,
				NodeRef:   contract.String(),
			},
			privKey: privKey,
			ref:     contract,
			role:    discoverNode.Role,
		}
	}
	return nodes, nil
}

func (g *Generator) activateNodeRecord(
	ctx context.Context,
	record *noderecord.NodeRecord,
	name string,
	nodeRecordProto insolar.Reference,
) (*insolar.Reference, error) {
	nodeData, err := insolar.Serialize(record)
	if err != nil {
		return nil, errors.Wrap(err, "[ activateNodeRecord ] Couldn't serialize node instance")
	}

	nodeID, err := g.artifactManager.RegisterRequest(
		ctx,
		*g.rootDomainRef,
		&message.Parcel{
			Msg: &message.GenesisRequest{Name: name},
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "[ activateNodeRecord ] Couldn't register request")
	}
	contract := insolar.NewReference(*g.rootDomainRef.Record(), *nodeID)
	_, err = g.artifactManager.ActivateObject(
		ctx,
		insolar.Reference{},
		*contract,
		*g.nodeDomainRef,
		nodeRecordProto,
		false,
		nodeData,
	)
	if err != nil {
		return nil, errors.Wrap(err, "[ activateNodeRecord ] Could'n activateNodeRecord node object")
	}
	_, err = g.artifactManager.RegisterResult(ctx, *g.rootDomainRef, *contract, nil)
	if err != nil {
		return nil, errors.Wrap(err, "[ activateNodeRecord ] Couldn't register result")
	}
	return contract, nil
}

func (g *Generator) addDiscoveryIndex(
	ctx context.Context,
	indexMap map[string]string,
	nodeRecordProto insolar.Reference,
) ([]genesisNode, map[string]string, error) {
	errMsg := "[ addDiscoveryIndex ]"
	discoveryKeysPath, err := filepath.Abs(g.config.DiscoveryKeysDir)
	if err != nil {
		return nil, nil, errors.Wrap(err, errMsg)
	}

	discoveryKeys, err := g.uploadKeys(ctx, discoveryKeysPath, len(g.config.DiscoveryNodes))
	if err != nil {
		return nil, nil, errors.Wrap(err, errMsg)
	}

	discoveryNodes, err := g.activateDiscoveryNodes(ctx, nodeRecordProto, discoveryKeys)
	if err != nil {
		return nil, nil, errors.Wrap(err, errMsg)
	}

	for _, node := range discoveryNodes {
		indexMap[node.node.PublicKey] = node.ref.String()
	}
	return discoveryNodes, indexMap, nil
}

func (g *Generator) createKeys(ctx context.Context, dir string, amount int) error {
	fmt.Println("createKeys, skip RemoveAll of", dir)
	// err := os.RemoveAll(dir)
	// if err != nil {
	// 	return errors.Wrap(err, "[ createKeys ] couldn't remove old dir")
	// }

	for i := 0; i < amount; i++ {
		ks := platformpolicy.NewKeyProcessor()

		privKey, err := ks.GeneratePrivateKey()
		if err != nil {
			return errors.Wrap(err, "[ createKeys ] couldn't generate private key")
		}

		privKeyStr, err := ks.ExportPrivateKeyPEM(privKey)
		if err != nil {
			return errors.Wrap(err, "[ createKeys ] couldn't export private key")
		}

		pubKeyStr, err := ks.ExportPublicKeyPEM(ks.ExtractPublicKey(privKey))
		if err != nil {
			return errors.Wrap(err, "[ createKeys ] couldn't export public key")
		}

		result, err := json.MarshalIndent(map[string]interface{}{
			"private_key": string(privKeyStr),
			"public_key":  string(pubKeyStr),
		}, "", "    ")
		if err != nil {
			return errors.Wrap(err, "[ createKeys ] couldn't marshal keys")
		}

		name := fmt.Sprintf(g.config.KeysNameFormat, i)
		err = makeFileWithDir(dir, name, result)
		if err != nil {
			return errors.Wrap(err, "[ createKeys ] couldn't write keys to file")
		}
	}

	return nil
}

func (g *Generator) uploadKeys(ctx context.Context, dir string, amount int) ([]nodeInfo, error) {
	var err error
	if !g.config.ReuseKeys {
		err = g.createKeys(ctx, dir, amount)
		if err != nil {
			return nil, errors.Wrap(err, "[ uploadKeys ]")
		}
	}

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrap(err, "[ uploadKeys ] can't read dir")
	}
	if len(files) != amount {
		return nil, errors.New(fmt.Sprintf("[ uploadKeys ] amount of nodes != amount of files in directory: %d != %d", len(files), amount))
	}

	var keys []nodeInfo
	for _, f := range files {
		privKey, nodePubKey, err := getKeysFromFile(ctx, filepath.Join(dir, f.Name()))
		if err != nil {
			return nil, errors.Wrap(err, "[ uploadKeys ] can't get keys from file")
		}

		key := nodeInfo{
			publicKey:  nodePubKey,
			privateKey: privKey,
		}
		keys = append(keys, key)
	}

	return keys, nil
}

func (g *Generator) makeCertificates(nodes []genesisNode) error {
	certs := make([]certificate.Certificate, len(nodes))
	for i, node := range nodes {
		certs[i].Role = node.role
		certs[i].Reference = node.ref.String()
		certs[i].PublicKey = node.node.PublicKey
		certs[i].RootDomainReference = g.rootDomainRef.String()
		certs[i].MajorityRule = g.config.MajorityRule
		certs[i].MinRoles.Virtual = g.config.MinRoles.Virtual
		certs[i].MinRoles.HeavyMaterial = g.config.MinRoles.HeavyMaterial
		certs[i].MinRoles.LightMaterial = g.config.MinRoles.LightMaterial
		certs[i].BootstrapNodes = make([]certificate.BootstrapNode, len(nodes))
		for j, node := range nodes {
			certs[i].BootstrapNodes[j] = node.node
		}
	}

	var err error
	for i := range nodes {
		for j, node := range nodes {
			certs[i].BootstrapNodes[j].NetworkSign, err = certs[i].SignNetworkPart(node.privKey)
			if err != nil {
				return errors.Wrapf(err, "[ makeCertificates ] Can't SignNetworkPart for %s", node.ref.String())
			}

			certs[i].BootstrapNodes[j].NodeSign, err = certs[i].SignNodePart(node.privKey)
			if err != nil {
				return errors.Wrapf(err, "[ makeCertificates ] Can't SignNodePart for %s", node.ref.String())
			}
		}

		// save cert to disk
		cert, err := json.MarshalIndent(certs[i], "", "  ")
		if err != nil {
			return errors.Wrapf(err, "[ makeCertificates ] Can't MarshalIndent")
		}

		if len(g.config.DiscoveryNodes[i].CertName) == 0 {
			return errors.New("[ makeCertificates ] cert_name must not be empty for node " + strconv.Itoa(i+1))
		}

		err = ioutil.WriteFile(path.Join(g.keyOut, g.config.DiscoveryNodes[i].CertName), cert, 0644)
		if err != nil {
			return errors.Wrap(err, "[ makeCertificates ] makeFileWithDir")
		}
	}
	return nil
}

func (g *Generator) updateNodeDomainIndex(ctx context.Context, nodeDomainDesc artifact.ObjectDescriptor, indexMap map[string]string) error {
	updateData, err := insolar.Serialize(
		&nodedomain.NodeDomain{
			NodeIndexPK: indexMap,
		},
	)
	if err != nil {
		return errors.Wrap(err, "[ updateNodeDomainIndex ]  Couldn't serialize NodeDomain")
	}

	_, err = g.artifactManager.UpdateObject(
		ctx,
		*g.rootDomainRef,
		*g.nodeDomainRef,
		nodeDomainDesc,
		updateData,
	)
	if err != nil {
		return errors.Wrap(err, "[ updateNodeDomainIndex ]  Couldn't update NodeDomain")
	}

	return nil
}

func getKeysFromFile(ctx context.Context, file string) (crypto.PrivateKey, string, error) {
	absPath, err := filepath.Abs(file)
	if err != nil {
		return nil, "", errors.Wrap(err, "[ getKeyFromFile ] couldn't get abs path")
	}
	data, err := ioutil.ReadFile(absPath)
	if err != nil {
		return nil, "", errors.Wrap(err, "[ getKeyFromFile ] couldn't read keys file "+absPath)
	}
	var keys map[string]string
	err = json.Unmarshal(data, &keys)
	if err != nil {
		return nil, "", errors.Wrapf(err, "[ getKeyFromFile ] couldn't unmarshal data from %s", absPath)
	}
	if keys["private_key"] == "" {
		return nil, "", errors.New("[ getKeyFromFile ] empty private key")
	}
	if keys["public_key"] == "" {
		return nil, "", errors.New("[ getKeyFromFile ] empty public key")
	}
	kp := platformpolicy.NewKeyProcessor()
	key, err := kp.ImportPrivateKeyPEM([]byte(keys["private_key"]))
	if err != nil {
		return nil, "", errors.Wrapf(err, "[ getKeyFromFile ] couldn't import private key")
	}
	return key, keys["public_key"], nil
}
