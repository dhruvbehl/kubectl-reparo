package cmd

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/sergi/go-diff/diffmatchpatch"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/yaml"
)

var (
	whereFlags []string
	setFields  string
	allNs      bool
	selector   string
	namespace  string
	dryRun     bool
	outputDiff bool
)

var rootCmd = &cobra.Command{
	Use:   "reparo <resource> [name]",
	Short: "Conditionally patch Kubernetes resources based on smart rules",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		resource := args[0]
		name := ""
		if len(args) > 1 {
			name = args[1]
		}

		fmt.Printf("\U0001f9d9 Resource: %s\n", resource)
		if name != "" {
			fmt.Printf("\U0001f50d Name: %s\n", name)
		}
		fmt.Printf("\U0001f3af --set: %s\n", setFields)
		fmt.Printf("\U0001f50e --where: %v\n", whereFlags)
		fmt.Printf("\U0001f3f7️ --selector: %s\n", selector)
		fmt.Printf("\U0001f30d --all-namespaces: %v\n", allNs)
		fmt.Printf("📦 --namespace: %s\n", namespace)
		fmt.Printf("\U0001f9ea --dry-run: %v\n", dryRun)
		fmt.Printf("📄 --output=diff: %v\n", outputDiff)

		kubeconfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
			clientcmd.NewDefaultClientConfigLoadingRules(),
			&clientcmd.ConfigOverrides{},
		)
		restConfig, err := kubeconfig.ClientConfig()
		if err != nil {
			fmt.Println("❌ Failed to load kubeconfig:", err)
			os.Exit(1)
		}

		disco, err := discovery.NewDiscoveryClientForConfig(restConfig)
		if err != nil {
			fmt.Printf("❌ Could not create discovery client: %v\n", err)
			os.Exit(1)
		}

		apiGroupResources, err := restmapper.GetAPIGroupResources(disco)
		if err != nil {
			fmt.Printf("❌ Could not get API group resources: %v\n", err)
			os.Exit(1)
		}

		resourceLower := strings.ToLower(resource)
		var gvr schema.GroupVersionResource
		found := false
		for _, group := range apiGroupResources {
			for version, resList := range group.VersionedResources {
				for _, res := range resList {
					if res.Name == resourceLower && !strings.Contains(res.Name, "/") {
						gvr = schema.GroupVersionResource{
							Group:    group.Group.Name,
							Version:  version,
							Resource: res.Name,
						}
						found = true
						break
					}
				}
			}
		}
		if !found {
			fmt.Printf("❌ Resource '%s' not found in API discovery\n", resource)
			os.Exit(1)
		}

		fmt.Printf("\U0001f50e Discovered GVR: %s\n", gvr.String())

		dynClient, err := dynamic.NewForConfig(restConfig)
		if err != nil {
			fmt.Printf("❌ Could not create dynamic client: %v\n", err)
			os.Exit(1)
		}

		ctx := context.TODO()
		var objs []unstructured.Unstructured

		if name != "" {
			ns := namespace
			if allNs {
				fmt.Println("⚠️ Skipping --all-namespaces: name was provided")
			}
			if ns == "" {
				ns = "default"
			}
			obj, err := dynClient.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				fmt.Printf("❌ Failed to get resource '%s': %v\n", name, err)
				os.Exit(1)
			}
			objs = append(objs, *obj)
		} else {
			ns := ""
			if !allNs {
				ns = namespace
				if ns == "" {
					ns = "default"
				}
			}
			list, err := dynClient.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				fmt.Printf("❌ Failed to list resources: %v\n", err)
				os.Exit(1)
			}
			objs = list.Items
		}

		filtered := []unstructured.Unstructured{}
		for _, obj := range objs {
			if len(whereFlags) == 0 || matchesAnyWhereBlock(&obj, whereFlags) {
				filtered = append(filtered, obj)
			}
		}

		fmt.Printf("\U0001f50d Matched %d resource(s) after --where filter:\n", len(filtered))
		for _, obj := range filtered {
			fmt.Printf("- %s/%s\n", obj.GetNamespace(), obj.GetName())
		}

		if len(filtered) > 1 && !dryRun {
			fmt.Print("❓ About to patch multiple resources. Proceed? [y/N]: ")
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Scan()
			answer := strings.TrimSpace(scanner.Text())
			if answer != "y" && answer != "Y" {
				fmt.Println("❌ Aborted by user.")
				return
			}
		}

		for _, obj := range filtered {
			if setFields != "" {
				original := obj.DeepCopy()
				updates := parseSetFields(setFields)
				err := setFieldsOnObject(&obj, updates)
				if err != nil {
					fmt.Printf("❌ Failed to set fields: %v\n", err)
					continue
				}

				if dryRun && outputDiff {
					origYaml, _ := yaml.Marshal(original.Object)
					newYaml, _ := yaml.Marshal(obj.Object)

					dmp := diffmatchpatch.New()
					diffs := dmp.DiffMain(string(origYaml), string(newYaml), false)
					diffs = dmp.DiffCleanupSemantic(diffs)

					linesA, linesB, lineArray := dmp.DiffLinesToChars(string(origYaml), string(newYaml))
					lineDiffs := dmp.DiffMain(linesA, linesB, false)
					lineDiffs = dmp.DiffCharsToLines(lineDiffs, lineArray)

					for _, d := range lineDiffs {
						switch d.Type {
						case diffmatchpatch.DiffInsert:
							fmt.Print("\033[32m+ " + d.Text + "\033[0m")
						case diffmatchpatch.DiffDelete:
							fmt.Print("\033[31m- " + d.Text + "\033[0m")
						case diffmatchpatch.DiffEqual:
							fmt.Print("  " + d.Text)
						}
					}
				} else if dryRun {
					fmt.Println("🧪 Dry-run result:")
					patch, _ := createPatchJSON(original, &obj)
					fmt.Println(string(patch))
				} else {
					patchBytes, _ := createPatchJSON(original, &obj)
					_, err = dynClient.Resource(gvr).Namespace(obj.GetNamespace()).Patch(
						ctx,
						obj.GetName(),
						types.MergePatchType,
						patchBytes,
						metav1.PatchOptions{},
					)
					if err != nil {
						fmt.Printf("❌ Patch failed for %s/%s: %v\n", obj.GetNamespace(), obj.GetName(), err)
					} else {
						fmt.Println("✅ Patched successfully.")
					}
				}
			}
		}
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringSliceVar(&whereFlags, "where", nil, "Conditions in key=value format. Multiple --where = OR; comma-separated = AND")
	rootCmd.PersistentFlags().StringVar(&setFields, "set", "", "Fields to update in key=value format")
	rootCmd.PersistentFlags().BoolVarP(&allNs, "all-namespaces", "A", false, "If true, search in all namespaces")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "Target namespace (overridden by -A)")
	rootCmd.PersistentFlags().StringVar(&selector, "selector", "", "Label selector (e.g., app=myapp)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "Preview changes without applying")
	rootCmd.PersistentFlags().BoolVar(&outputDiff, "output", false, "Show YAML diff output on dry-run")
}
