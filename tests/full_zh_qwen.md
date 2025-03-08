# 基于FPGA的实时点云配准框架，具有超高速和可配置的对应匹配能力

邓琪，IEEE研究生会员，孙浩，IEEE会员，舒宇浩，肖建中，姜伟雄，王辉，哈亚君，IEEE高级会员

摘要—点云配准是基于LiDAR的定位和建图系统中的关键组件，然而现有的实现因对应搜索效率低下导致速度受限。为了解决这一挑战，我们提出了一种基于FPGA的实时点云配准框架，该框架具有超高速和可配置的对应匹配能力。该框架包含三个关键创新：首先，我们开发了一种新颖的Range-Projection Structure (RPS)，它将无结构的LiDAR点组织为矩阵形式，实现了高效的点定位，并将附近的点分组到连续的内存段以加速访问；其次，我们引入了一种高效的多模式对应搜索算法，利用RPS结构缩小搜索区域，消除冗余点，并通过结合激光雷达特有的激光通道信息支持多种对应匹配类型；第三，我们设计了一个可配置的超高速基于RPS的对应搜索（RPS-CS）加速器，其特点是高性能的RPS-Builder用于快速结构构建；高度并行的RPS-Searcher用于快速对应搜索。通过动态优化技术进一步提升了性能。为了提高效率和可配置性，提出了一种分层策略和流水线批量处理模块。实验结果表明，RPS-CS加速器相比最先进的FPGA实现，实现了7.5倍的速度提升和17.1倍的能效提升，而所提出的框架在处理64线激光雷达数据时实现了20.1 FPS的实时性能。

索引词——FPGA，硬件加速，对应匹配，点云配准，基于激光雷达的SLAM系统

# I. 引言

点云配准是基于激光雷达的同时定位与地图构建（LSLAM）系统中的关键组件，核心是计算一个变换矩阵以将源点云与目标点云对齐，如图1所示。最先进的配准算法[1]的流程图如图2所示，该算法输入两种类型的特征点并计算最优变换矩阵。高效且准确的配准实现一直是诸如自动驾驶和机器人等领域的重要研究课题。

为了提高配准算法的准确性，必须采用具有更多激光通道和更高帧率的先进激光雷达系统。例如，3D激光雷达（Robosense Ruby Plus）已升级到128线激光，以20赫兹的扫描频率每秒生成4,608,000个点。然而，这种升级不可避免地会导致计算耗时显著增加。配准算法[2][3]的执行时间。图3所示的性能分析结果显示，即使使用来自64线激光雷达的点云，配准算法[1]在车载嵌入式CPU上也只能实现每秒不到2帧（FPS）的处理速度，这远远不能满足实时要求。性能瓶颈在于对应点搜索任务，该任务占总配准时间的91.6%。

先前的工作通过优化搜索结构、搜索算法和硬件加速器，在点云配准中显著加速了对应点搜索。一些研究[4]–[6]将稀疏且不均匀的点云组织成基于树或基于体素的搜索结构，从而实现高效的K近邻对应（KNNC）搜索。然而，这些结构往往难以在快速定位点和高效提取邻近点之间取得平衡，特别是在高度不均匀的点分布情况下。此外，它们难以保留关键三维结构和几何信息，限制了其处理几何对应关系（如平面-NN对应（PNN-C）和边缘-NN对应（ENN-C））的能力。搜索算法也面临实现并行处理和高效处理几何对应关系的挑战。近似方法[7]–[10]减少了搜索时间但牺牲了准确性，而数据访问优化方法[11][12]（如缓存和并行聚合）虽然提高了效率，但增加了重新排序和调度处理的开销，限制了其实时应用。硬件加速器，包括GPU和FPGA[13]–[15]，进一步提升了性能。GPU利用并行性加速KD树构建和KNN搜索，但能耗高，降低了能源效率。基于FPGA的解决方案[5][6]更加节能，采用软硬件协同优化策略将任务分配给硬件和软件。然而，它们通常无法高效处理大规模数据缓存或为多种几何对应关系（如PNN-C和ENN-C）提供可配置支持，而这些对于点云配准至关重要。![](images/9d3da5e7f79eca50495723906b3d449d3dec80312eb2192ca2754f2c26ebbff3.jpg)图1. 点云配准算法示意图。

为了解决这些问题，我们提出了一种基于FPGA的实时LiDAR点云配准框架，该框架具有超快速和可配置的对应点搜索能力（注：原文中“confgurable”应为“configurable”）。我们做出了以下三项贡献。

• 一种新颖的搜索结构（RPS），用于高效查询点定位和邻近点访问。RPS结构根据LiDAR点的投影坐标和距离值（与LiDAR的距离）将无序且分布不均的LiDAR点组织成矩阵形式。为了便于快速访问邻近点，RPS将具有相似距离值和投影坐标的点分组到连续内存块中，大幅降低搜索复杂度。

• 利用RPS结构的高效多模式匹配搜索算法。利用RPS提供的空间组织，所提出的算法能够高效地缩小搜索区域并消除大量冗余点。此外，通过结合LiDAR特有的激光通道信息，该算法支持多模式匹配搜索，实现了不同类型对应点的快速准确搜索。

• 超快速且可配置的基于RPS的对应点搜索加速器（RPS-CS加速器）。RPS-CS框架包含两个关键组件：(1) RPS-Builder：高性能加速器，旨在快速从LiDAR点云构建RPS结构。(2) RPS-Searcher：高度并行化且可配置的加速器，用于快速对应点搜索。通过动态RPS缓存机制，自适应将外部内存中的邻近点预加载到片上内存中，以提高内存访问效率。此外，为了增强搜索效率和可配置性，RPS-Searcher采用了一个流水线批处理模块，将不同数量的点聚合成固定大小的数据包。![](images/740f908ac4dd0dd77adc3e3d2c0189295350bd4d1fa97f82d3bacf06e099293b.jpg)  
Fig. 2. Flowchart of the point cloud registration algorithm.  

![](images/90575386dc1ef68fb8d44cceea239ae4461330e63c614e01e7a8645a12e43963.jpg)图3. 配准算法在CortexA53上的运行时间分析结果。

本文其余部分组织如下。第二部分介绍点云配准算法的背景知识和相关工作。第三部分展示了所提出的框架概述。第四部分介绍了RPS结构及构建加速器。第五部分展示了基于RPS的快速且可扩展的对应点搜索算法和加速器。第六部分给出了实验结果和分析。最后，第七部分总结了结论和未来的工作。

# II. 背景和相关工作

# A. 配准算法的定义与分析

在激光SLAM（L-SLAM）系统的配准任务中，当前帧的点云定义为源点云，记作$\mathbf{P}$，而前一帧的点云定义为目标点云，记作$\mathbf{Q}$，其中$\mathbf{P}, \mathbf{Q}\in\mathbb{R}^3$。通常，一帧的点云是指LiDAR传感器在其360度水平旋转扫描过程中每秒捕获的3D点的集合。

配准的目标是通过变换矩阵$T=(R,t)$估计一个刚体运动，从而最大化源点云$T\cdot P$与目标点云$\mathbf{Q}$之间的重合度。这里，$R$表示旋转矩阵，$t$表示平移向量。图1说明了配准算法。

图2给出了最先进的配准算法[1]的流程图，该算法接收两种类型的特征点并计算最优变换矩阵。在此背景下，源点云中与平面和边界相关的特征点分别标记为$P_{\mathcal{H}}$和$P_{\mathcal{E}}$。相应地，目标点云中与平面和边界相关的特征点分别标记为$Q_{\mathcal{H}}$和$Q_{\mathcal{E}}$。![](images/5621356d277ccf8ea5b63b304c83ceab5b05749ab194f2e567adf35f8e79f8a7.jpg)图4. 对应点搜索的示意图。三个带蓝圈的红点组成平面查询点$P_{\mathcal{H}}^{i}$的对应点。两个带蓝圈的绿点组成边缘查询点$P_{\mathcal{E}}^{i^{\prime}}$的对应点。

通常，平面点位于墙上，而边缘点位于角落，如图4所示。配准算法包含三个主要模块。

1) 构建搜索结构模块：第一个模块专注于构建搜索结构，以高效地组织目标点云，以便后续模块能快速准确地搜索对应点。在SLAM算法中常用的一种搜索结构是K-Dimensional Tree（KD树），这是一种空间划分技术，将点云组织成层次化的树形结构，便于快速搜索操作。因此，许多先前的工作，例如[1]、[16]、[17]，采用KD树来组织$Q_{\mathcal{H}}$和$Q_{\mathcal{E}}$特征点。$N_Q$表示用于构建搜索结构的目标点云中的点数。

2) 搜索对应点模块：系统的第二个模块旨在建立源点云和目标点云之间的对应关系。对应关系的质量直接影响配准算法的准确性。利用前一个模块构建的KD树搜索结构通过以下三个步骤搜索对应关系。

首先，通过初始估计的变换矩阵 \( T_{init} = (R_{init}, t_{init}) \) 将源点云中的点转换到目标点云的坐标系中，其中 \( R_{init} \) 和 \( t_{init} \) 分别表示旋转矩阵和平移向量。源点 \( P_{\mathcal{H}}^{i} \) 和 \( P_{\mathcal{E}}^{i} \) 被定义为查询点，查询点的数量记为 \( N_{P} \)。

$$
{P_{\mathcal{E}(\mathcal{H})}^{i}}^{\prime} = T_{init} \cdot P_{\mathcal{E}(\mathcal{H})}^{i} = R_{init} P_{\mathcal{E}(\mathcal{H})}^{i} + t_{init}
$$

其次，为每个查询点搜索对应关系。如图2所示，对应关系分为三类：KNN-C、PNN-C 和 ENN-C。

1) KNN-C 通常涉及识别每个查询点的 K 个最近邻作为对应点，这是基础功能，在机器人应用中广泛使用。
2) PNN-C 指的是识别三个最近的平面特征点组成对应的平面。在图4中，查询平面点 \( P_{\mathcal{H}}^{i} \) 的 PNN-C 点由 \( (Q_{\mathcal{H}}^{j}, Q_{\mathcal{H}}^{m}, Q_{\mathcal{H}}^{l}) \) 组成，其中 \( Q_{\mathcal{H}}^{j} \) 表示在目标点云中距离 \( P_{\mathcal{H}}^{i} \) 最近的点。

注意：源文本中包含一些无法理解的乱码部分（如 "translation tvreacntsofro. rImne de qtuoa (a1n),d f,e raetusrpee ctpiovienltys."），这些内容已被省略。{\mathcal{H}},\,Q_{\mathcal{H}}^{m}$ 表示距离 $P_{\mathcal{H}}^{i}$ 最近的点，位于与 $Q_{\mathcal{H}}^{j^{\dagger}}$ 相邻的两个激光通道上，而 $Q_{\mathcal{H}}^{l}$ 表示与 $Q_{\mathcal{H}}^{j}$ 位于同一激光通道上距离 $P_{\mathcal{H}}^{i}$ 第二近的点。PNN-C 点 $(Q_{\mathcal{H}}^{j},\,Q_{\mathcal{H}}^{m},\,Q_{\mathcal{H}}^{l})$ 定义了 $P_{\mathcal{H}}^{i'}$ 对应的平面，如图中蓝色三角形所示。

3) ENN-C 指的是通过识别最近的两个边缘点作为查询边缘点对应的边缘线。在图4中，查询边缘点 $\overline{P_{\mathcal{E}}^{i'}}$ 的 ENN-C 点由 $(Q_{\varepsilon}^{j},\,\bar{Q}_{\varepsilon}^{m})$ 组成，定义了 $P_{\mathcal{E}}^{i'}$ 对应的边缘线，如图4中蓝色线条所示。

第三步，计算源特征点与匹配点之间的对应距离。到其对应的边缘线和平面的距离分别表示为 $\mathbf{d}_{\mathcal{E}}^{i}$ 和 $\mathrm{d}_{\mathcal{H}}^{i}$，如图4中所示。需要指出的是，只有在特定范围内的邻居才被认为是有效的。如果最近邻点与查询点之间的距离超过预定义的阈值，则该查询点被视为异常值（outlier）并排除，以确保其不影响配准结果。这个阈值范围定义为 $r_{in}$，确保所有相关的邻居都被包含在内。包含在内，而超出$r_{in}$的点被忽略。

3) 运动估计模块：第三个模块专注于使用对应点进行运动估计。该过程首先通过制定非线性最小二乘函数来衡量配准误差，并采用Levenberg-Marquardt算法迭代地将初始变换矩阵$T_{init}$细化为最优矩阵$T_{opt}$。这个最优矩阵确保了变换后的源点云（表示为$T_{opt}\cdot P$）与目标点云$Q$之间的最佳对齐，如图1所示。此外，如[1]所述，迭代次数设为2次，这意味着每次配准过程包括一次构建搜索结构和两次执行对应点搜索。

# B. 相关工作

在本小节中，我们介绍点云配准算法在搜索结构优化、搜索算法改进和硬件加速方面的相关工作。

在搜索结构优化方面，树结构，特别是KD树、局部敏感哈希和溢出树，常用于点云配准。根据比较研究[9]，KD树在准确性、构建时间、搜索时间和内存使用方面具有显著优势。然而，在处理自动驾驶场景中常见的稀疏且分布不均的点云时，构建和搜索过程的效率显著下降。为了有效应对这一问题为了管理这样的点云，已经开发了诸如双分割体素结构（DSVS）[5]和占用感知体素结构（OAVS）[6]、[12]、[18]等新颖的空间分割结构。这些方法将点云分割成三维立方空间或体素，消除空体素并持续细分被占用的体素，随后根据体素的哈希值（Hash值）组织这些点。尽管这些方法在构建时间和内存使用方面表现出竞争优势，但每个体素中点的数量以及相邻体素的数量不固定，导致搜索速度相对较慢。

在搜索算法优化方面，一些研究[7]、[11]、[19]、[20]采用了近似搜索方法，如范围搜索或概率分布搜索，这可以将搜索时间减少多达两个数量级。然而，这些方法往往在准确性和鲁棒性上有所妥协。为了提高搜索效率而不牺牲准确性，一些研究[12]、[21]–[23]专注于优化数据访问策略。探索了诸如通过缓存搜索结果以提高后续搜索的命中率或通过聚合点实现并行访问等技术。然而，这些方法引入了重新排序或调度点的额外时间开销。此外，利用对应搜索任务中固有的特征（特别是对应点的局部性和几何特性）至关重要，以充分利用这些特征。极大地提高了搜索效率。

在硬件加速方面，FPGA和GPU已被广泛用于加速点云配准算法。研究[13]、[24]、[25]展示了使用高度并行策略在GPU上高效构建KD树和进行KNN最近邻搜索。然而，基于GPU的方法通常能耗较高，降低了其能效。相比之下，基于FPGA的解决方案提供了更高的能效。一些工作[5]、[8]、[9]、[26]–[28]采用可重用的多级存储缓存、基于关键帧驱动调度以及高度并行的排序和选择电路模块来提高实时性能。尽管取得了这些进展，大多数努力仍集中在优化搜索加速器，而忽视了其他组件的执行时间和数据传输的时延开销，限制了整体效率。为了解决这些问题，一些研究[6]、[15]将点云配准算法划分为硬件和软件组件，利用协同优化策略减少冗余搜索操作并提高效率。同时，其他工作[10]、[14]、[29]在算法、架构和缓存级别上优化空间结构构建和KNN最近邻搜索，实现了高度可配置和超快的KNN加速模块。然而，大多数依赖于近似搜索技术（Approximate Search），过滤掉大量邻近点，无法满足激光SLAM系统（LSLAM）严格的精度要求。此外，几乎所有现有方法都未能结合搜索结构的几何特性，导致平面和边缘匹配搜索效率低下。

# 第三节：所提出的配准框架设计概述

本节概述了所提出的基于RPSCS的软硬件协同设计点云配准框架。基于第二节A部分的分析，我们介绍了该框架的关键组件，包括RPS结构、基于RPS的最近邻搜索算法、RPS-CS加速模块以及协同配准工作流。![](images/baccb50115b09e64e897178f2356b7d6dd3dee1001674ac045901684583f8bb1.jpg)图5. 通过RPS结构对点云进行分割的示例。相同颜色的点是从相同的激光通道测量的。

# A. RPS结构和RPS-CS算法概述

为了利用对应点的局部性和几何特征，我们提出了一种新型搜索结构，称为RPS，如图5所示。RPS结构将点云数据组织成一种高效的格式，以便进行对应点搜索，具体工作流程如下：

• 首先，RPS结构将点云投影为矩阵格式，其中行对应激光通道，列对应水平旋转角度。这个过程创建了数据的结构化表示，图5中的蓝色网格表示投影位置。
• 其次，根据点到LiDAR传感器的距离（范围值），将投影点划分到不同范围尺度区间。每个范围尺度对应特定的范围值区间，允许使用行、列和范围尺度索引精确定位点。图5展示了范围值与范围尺度之间的关系。
• 第三，我们将点云矩阵划分为一组范围尺度分割域（RSSD），其中具有相似范围尺度的点被通过计数排序算法进行重新排序到连续内存区域。最终的RPS结构由两个关键组件组成：RPS-Points，用于存储重新排序的点并按范围尺度分组；RPSIndex，记录每个范围尺度的起始索引。图7提供了该结构的详细示例。

基于RPS结构，我们提出了一种高效且灵活的对应点检测算法，称为RPS-CS，以在指定的搜索半径$r_{in}$内检测多种类型的对应点。RPS-CS算法按以下五个步骤执行：

• 首先，通过计算查询点的水平角度和激光通道对应的行和列索引，以及根据其范围值定位查询点的RPS坐标。范围尺度索引是通过预定义的查找表（LUT）查得的。
• 其次，根据查询点的位置和搜索半径$r_{in}$确定搜索区域的空间范围，如公式（5）所示。该区域被表示为以下一组![](images/4f7d464d8f5cba0acbc661dba52d5dbb8318a07e3b3e48faa6c231754cd6c77b.jpg)图6. 所提出的RPS-CS加速器及基于RPS-CS的软硬件协同设计注册框架的示意图。

RPS-Index对，其中每对指定RPS-Points中的一组点子集。

• 第三，从RPS结构中提取候选对应点。对于每个有效的RPS-Index对，点可并行从RPS-Points中检索，因为具有相似范围尺度和投影位置的点已被重新排序到连续的内存块中。• 第四，算法使用高度并行的K选择方法从候选点中确定KNN-C。如果搜索目标仅限于KNN-C，则直接输出结果。否则，根据特定几何特征进一步筛选候选点。最后，使用筛选后的点搜索其他类型的对应关系，如PNN-C或ENN-C。这些额外的对应关系通过基于激光通道的快速条件K选择方法计算，如算法2所示（见算法2）。

# B. 基于RPS-CS的注册框架概述

除了优化搜索结构和搜索算法外，我们还提出了一种软硬件协同设计的注册框架，以进一步提升性能。该框架建立在一个异构系统架构上，结合高性能处理系统（PS）与用户可编程逻辑单元（PL）在同一FPGA板上。框架的结构设计和操作工作流程如图所示。如图6所示。

在PL侧，我们实现了一个RPS-CS加速器，包含两个主要组件：RPS-Builder和RPS-Searcher。

• RPS-Builder：该组件负责使用数据流架构构建RPS结构，由图6中的蓝色块表示。对于每个RSSD中的点，模块执行以下任务：将目标点投影到矩阵格式，将点组织成基于RSSD的流，统计每个范围层级内的点数，计算每个范围层级的起始索引，并相应地重新排序点。这个并行化过程确保高效生成RPS结构。

• RPS-Searcher：该组件旨在以内存效率和并行性为重点，进行快速且可配置的对应关系搜索，由图6中的绿色块表示。搜索过程被组织成七个模块：将查询点投影到RPS结构中；缩小搜索区域；提取RPS-Index对；并行提取每对中的点；将邻近点（near points）聚合为批次；使用高度并行化的K选择电路搜索K近邻候选点（KNNC）；使用基于激光通道的K选择电路识别P近邻候选点（PNN-C）和E近邻候选点（ENN-C）。这种模块化设计确保了各种对应关系搜索任务的高效性和适应性。

此外，RPS-Parameter组件通过配置RPS-CS加速器中的所有模块，支持多模式工作模式和自定义优化功能（function）。注意：原文中“datafow architecture”应为“dataflow architecture”，“funct”应为“function”，建议在注释中提醒。

（注：标点符号已统一为全角符号，长句适当分段以提升可读性。）它实现了RPS-Builder和RPS-Searcher组件之间的切换，并根据不同的对应类型（correspondence types）调整参数，例如：搜索区域大小。

在PS端（Processing System），框架管理点云数据存储、RPS结构信息、运动估计以及协作配准工作流程的整体控制，如第II-A节所述。

PS端和PL端（Processing Logic）之间的所有接口均使用流式传输FIFO端口实现，并应用数据打包以提高传输效率。在加速器内部，数据以定点格式表示，而在加速器外部，则以浮点格式表示。数据类型转换过程如图6的端口图标所示。此外，FPGA的寄存器传输级（RTL）模型由Xilinx Vitis高层次综合（HLS）工具生成。同时，我们还集成了一个动态电压频率调节（DVFS）模块[30]以优化能量效率。

# 第四节 高效构建RPS搜索结构

在本节中，我们首先介绍构建RPS结构的详细信息。然后，我们提出RPS-Builder加速器的硬件设计方案。![](images/2f26f236b1bcb500c0aa0bcc88e1ee28611c897a4106d2d802a81d424951c1ba.jpg)(c) 通过计数排序算法生成RPS结构  
图7. 构建RPS搜索结构的示例。“R”表示范围尺度。

# A. 构建RPS搜索结构

由于RPS搜索结构根据投影位置和范围值对点云进行分段，构建RPS结构分为三个阶段。首先，通过对点云进行投影来分割点云。如图7（a）所示，原始点云被组织成一个基于RSSD方式的矩阵，使用低复杂度但高精度的投影方法[30]。其次，对点云矩阵按范围值进行划分。我们按RSSD方式遍历点云矩阵，计算每个点的范围值。这些范围值代表了每个点到LiDAR传感器的距离。基于这些值，我们将每个预设区域内的点归类为不同的范围尺度，如图7（b）所示。最后，通过计数排序算法生成RPS搜索结构。RPS结构包含两个关键组件：重新排序的点（RPS-Points）和每个范围尺度的起始索引（RPS-Index）。相邻RPS-Index值的数值差表示各范围尺度内的点数。图7（c）展示了RPS-Points和RPS-Index的示例结构。

1) 通过对点云进行投影来分割点云：考虑到一帧点云是在LiDAR传感器进行360°的水平旋转扫描时捕获的，可以将点云投影到维数为$V \times H$的矩阵中。这里，$V$表示激光通道的数量，而$H$表示每个激光通道在360°水平旋转过程中获得的测量次数。$H$的值由公式$H = \frac{360}{\Delta\alpha}$（$\Delta\alpha$表示水平旋转角度分辨率）计算得出。

给定一个点$p(x,y,z)$，行和列索引$(\nu,h)$利用公式（2）计算得出，其中$\Delta\omega$是垂直方向上相邻激光通道之间的平均角度分辨率。<html><body><table><tr><td>Algorithm 1 Obtain RPS-Points by counting sort algorithm</td></tr><tr><td>Input: Point cloud matrix PCM[H][V] Input:RPS-Index RPSI[H][M] Output: RPS-Points RPSP[No]</td></tr><tr><td>1: reset range scale occupancy count RSOC[H][M] = 0 2:reset reorderindexRI=0 3: for i ∈ [O,H) do >eachRSSDinPCM</td></tr><tr><td>4: for j ∈ [o, V) do > each point in RSSD</td></tr><tr><td>5: obtain the range scale R of PCMij</td></tr><tr><td>6: if R ∈ [o, M) then > filter empty elements</td></tr><tr><td>7: RI ←RPSI[i][R] + RSOC[i][R]</td></tr><tr><td>8: RSOC[i][R]←RSOC[i][R]+ 1 9: RPSP[RI] ← PCMij 10: end if 11: end for 12: end for</td></tr></table></body></html>$$
\begin{array}{l}{\nu=\arctan(z/\sqrt{(x^{2}+y^{2})})/\Delta\omega}\\ {h=\arctan(y/x)/\Delta\alpha}\end{array}
$$

为了高效计算 $(\nu,h)$，我们采用了一种基于先前研究 [30] 的改进方法，该方法利用两个查找表（LUT）快速且准确地确定行和列索引。首先，根据公式 $a_{\nu}=z^{2}/(x^{2}+y^{2})$ 和 $a_{h}=y/x$ 分别计算垂直和水平方向的查找值 $a_{\nu}$ 和 $a_{h}$。然后使用这些值从两个预定义的 LUT 中检索相应的行和列索引。LUT 特别设计以考虑激光通道的垂直角度和水平旋转角度，以确保准确性和计算效率。

图 7 (a) 给出了一个点云矩阵的例子，其中黄色三角形的坐标 $(\nu,h)$ 被确定为 (5, 2)。

2) 按照距离值划分点云：考虑到激光雷达点分布稀疏且不均匀，通常在较近的距离处更密集，在较远的距离处更稀疏，我们引入了距离尺度的概念来进一步划分点云矩阵。这个过程包括两个主要步骤：

$$
r=\sqrt{x^{2}+y^{2}+z^{2}}
$$

计算距离值：对于点 $p(x,y,z)$，我们首先通过公式 (3) 计算距离值 $r$。该值量化了点 $p$ 到 LiDAR 传感器的距离。

使用距离尺度进行分割：接下来，我们引入距离尺度将距离空间划分为多个区间。假设 LiDAR 的最大检测距离为 $r_{max}$ 米，我们将距离空间划分为 $M$ 个距离尺度。$r_{max}$ 的值通常从 LiDAR 的数据表中获得，而 $M$ 是根据点云分布特性和实验分析确定的。随后，建立一个非均匀距离尺度查找表（RLUT，Range LUT），将不同的距离值与其对应的距离尺度建立对应关系。图 5 描述了距离值 $(r)$ 与距离尺度 $(R)$ 之间的关系，其中最大检测距离 $r_{max}$ 设为 20 米，$M$ 为 4。每个 $R_{i}$ 对应一个区间。[r_{i},\,r_{i+1})$ . In Fig. 7 (a), the range scale $R$ of the yellow triangle is 3.  

![](images/a2df41529cc25aa10aca583fe0e05b426eb1f5978080e464fc348fa3a72d3599.jpg)图8. RPS-Builder加速器的硬件架构。

因此，我们可以获得一个增强的点云矩阵，其属性为$(x,y,z,r,R)$。此外，空元素填充为$(0,0,0,-1,-1)$。

3) 构建RPS搜索结构：采用流式计数排序方法。为了构建RPS搜索结构，我们利用基于点云矩阵和范围层级的流式计数排序方法。首先，定义RSSD以将点云矩阵分割为区域，每个RSSD包含$N_{s d}$列。图7 (a-b)（分别展示不同RSSD的分布情况）展示了以不同颜色区分的八个RSSD。其次，计数排序方法应用于每个RSSD，分为三个步骤：

1) 统计各范围层级的点数。前三个RSSD的统计结果如图7 (c)所示。
2) 计算RPS索引，它指示每个范围层级在重新排序的RPS-Points中的起始位置。这是通过累加统计结果来实现的，如图7 (c)所示。
3) 根据范围层级和RPS索引重新排序点以生成RPS-Points。算法1详细描述了这个过程，图7 (c)提供了一个示例。

最终得到的重新排序后的目标点云，称为RPSPoints（RPS-Points的简写形式），保留了与原始点云相同的大小，但其属性扩展为$(x,y,z,r,R,\nu)$。RPS索引的大小为$M \times (H/N_{s d})$，反映了根据范围层级和RSSD对点云的分割。

针对ENN-C的特殊处理：边缘特征点需要表现出更高的曲率值。如文献[30]所述，从同一扫描激光束获得的两个连续边缘特征点之间的距离大于5个单位。为了优化ENN-C搜索结构，我们将转换后的点云矩阵大小调整为$M \times H/(5N_{sd})$。这一修改显著减少了ENN-C搜索过程中的搜索空间，提高了计算效率。

# B. 构建RPS的硬件加速器

在本小节中，我们介绍在所提出的RPSBuilder加速器中实现的硬件设计和优化策略。我们专注于在减少硬件资源占用的同时提升性能。

在架构层面，我们为加速器设计了一个高效的任务层级的流水线结构，如图8中的红线所示。该过程首先将目标点投影到矩阵格式。然后这些点按照基于RSSD的顺序进行调度，并重新排序到RPSPoints数组中。

为了简化系统架构图，我们设置参数为$M=72$，$H=1800$，$V=64$。此外，RCounter和R-First-Index模块合并至PointsReorder模块。因此，RPS-Builder加速器被分为三个独立的模块：点投影（PP）、RSSD顺序调度（RWS，RSSD-Wise Scheduler）和点重排序（PR）。

1) 点投影模块：该模块旨在优化硬件资源效率，同时保持高吞吐量的流水线性能。线性架构。我们采用多分辨率LUT（查找表）策略来实现这种平衡，如图8所示。

为了高效查找包含1800个条目的大型LUT中的列索引，我们使用基于坐标的多分辨率LUT方法，该方法包括四个关键步骤：首先，我们通过除法运算计算每个点的水平角度查值$a_{h}$；其次，我们使用一个紧凑的、4条目的坐标轴LUT，通过$x$和$y$值确定水平角度所在的象限；第三，我们使用30条目的粗粒度LUT根据$a_{h}$细化查找区域；最后，在这个缩小的区域内，通过细粒度LUT进行查找以确定列索引$h$。

考虑到范围比例LUT和行LUT相对较小，我们分别为它们各自的功能实现独立的LUT。这两个LUT的大小分别配置为72个和64个条目，对应于范围比例的数量和激光通道数量。

2) RSSD顺序调度模块：尽管采用了高精度LUT投影方法，但投影点的顺序并不严格遵循RSSD顺序。无序分布的点影响了我们加速器的效率。因此，我们开发了一个高效的RSSD调度模块(RWS)，利用紧凑的点云矩阵缓冲区（Point Cloud Matrix Buffer，PCMB）和高吞吐量流水线架构，确保点输出结果严格遵循RSSD顺序。该设计面临三个主要挑战：

• 确定PCMB的大小：经过广泛的实验分析，我们将PCMB列大小设置为8。这个大小是最优以防止覆盖新输入的点而尚未输出的点。

• PCMB输入和输出的管道调度：我们通过比较写入和读取列动态管理PCMB的读写操作，如图8中的绿线所示。当输出队列过载时，此机制会停止新点的输入，从而保持管道结构的高性能。

• 防止覆盖待输出的点：实现了一个1位投影标志（PF，Projection Flag）数组，大小为$64\,\times\,1800$，用于跟踪元素是否已被投影。如果已投影，则输出PCMB中的数据；否则，输出范围尺度（range scale）为-1的默认数据。此外，如图8中的蓝线所示，我们在管道结构中集成了PF数组重置过程以优化延迟。此过程持续重置输入列之后的第4到第8列中的元素状态。

3) 点重排序模块：该模块通过高吞吐量管道结构（high-throughput pipeline）有效实现了计数排序算法。如图8所示，我们的设计包含72个点计数器、72个范围尺度首次索引（RSFI触发器，flip-flop）和72个范围尺度占用计数（RSOC）计数器。数字72对应设计参数M的取值。这些组件共同确保了系统的高效运行。这些组件有助于重排序RSSD中的所有64个点，从而确保高效的并行处理。

第五章 快速可配置的对应搜索

在本节中，我们首先介绍基于RPS结构搜索KNN-C、PNN-C和ENN-C的三种过程。随后，我们提出了RPS-Searcher加速器的硬件实现。

# A. 基于RPS的多类型对应搜索

从第二节-A可知，PNN-C被确定为最复杂的对应类型，因为它涵盖了KNN-C和ENN-C的搜索过程。因此，我们以PNN-C为例，说明基于RPS结构和搜索范围𝑟𝑖𝑛的搜索过程。

对于查询点$P_{\mathcal{H}}^{i}\,^{\,\,\,\overline{{\prime}}}$，PNN-C包含点$(Q_{\mathcal{H}}^{j}, Q_{\mathcal{H}}^{m}, Q_{\mathcal{H}}^{l}) \in Q_{\mathcal{H}}$。搜索过程涉及五个步骤，如图9所示：

1) 确定查询点的RPS位置，计算$P_{\mathcal{H}}^{i}$的RPS位置$(\nu_{i}, h_{i}, R_{i})$，包括行索引$\nu_{i}$、列索引$h_{i}$和范围参数$R_{i}$，如第四节所述的那样。例如，在图9(a)和(b)部分，查询点（红星）的RPS位置为$(3, 1, 2)$。![](images/93a71b7500894d792ca424f2c70304ea6be8151b2480a9bd2bde961c4e0446c2.jpg)图9：基于RPS的对应关系搜索示例。这些图修改自【31】。

2) 计算搜索区域：定义搜索区域为一个以$(\nu_{i},h_{i},r_{i})$为中心的立方体，边长为$2r_{in}$。首先，使用基于$r_i$的公式(4)和查找表（LUT）确定搜索区域尺寸$h_{sr}=[h_{min},h_{max}]$和$R_{sr}=[R_{min},R_{max}]$。例如，红色星的搜索区域示例为$h_{sr}=[0, 2]$和$R_{sr}=[2, 3]$，如图9(a)-(b)所示。其次，将这些尺寸映射到RPS-Index数组索引上，使用公式(5)进行定义。例如，图9(c)定义搜索块为区间[2, 4)，[6, 9)，和[10, 12)。

$$
\begin{array}{r l}
& h_{min}=h_i - \arcsin{(r_{in}/r_i)}/{\Delta\alpha}\\ 
& h_{max}=h_i + \arcsin{(r_{in}/r_i)}/{\Delta\alpha}\\ 
& R_{min}=R_LUT[r_i - r_{in}]\\ 
& R_{max}=R_LUT[r_i + r_{in}]
\end{array}
$$  

$$
\begin{array}{r l}
& S_{B_{min}}=RPSI[h_c*M+R_{min}]\\ 
& S_{B_{max}}=RPSI[h_c*M+R_{max}+1]
\end{array}
$$  

3) 提取搜索区域内的候选对应点：根据搜索块从RPS-Points数组中提取相关点，并将这些点作为候选对应点提取。图9(d)用红色矩形突出显示了搜索块中的点。这种结构化的RPS-Points数组允许在每个块内并行提取点。

4) 执行搜索

$$...$$。在候选对应点中为KNN-C进行搜索：在每个搜索块内，计算查询点到候选点的平方距离以选择KNN点，并将这些点作为候选几何点流式传递到后续步骤。使用优先队列[30]在所有候选点中搜索KNN-C，确定最近的点为$Q_{\mathcal{H}}^{j}$。

5) 从候选几何点中搜索PENN-C。不同于基于优先队列的K选择方法[30]，我们提出了一种快速条件K选择方法，利用激光通道高效选择$Q_{\mathcal{H}}^{m}$和$Q_{\mathcal{H}}^{l}$。

算法2 基于激光通道的并行条件优先队列K选择方法——用于流式批量点
输入：候选几何点$\overline{{\operatorname{GP}[N_{g}]}}$，带有范围属性`.r`、行索引`.ν`和RPS索引`.i`；最近点$Q_{j}$
输出：PNN-C：$Q_{l}$和$Q_{m}$
1: 并行遍历 $i\in[0,N_{g})$ ⊲ 针对GP中的所有点
2: 列索引差距：$G_{\nu}[i] \leftarrow \mathrm{abs}(GP[i].ν - Q_{j}.ν)$
3: RPS索引差距：$G_{i}[i] \leftarrow \mathrm{abs}(GP[i].i - Q_{j}.i)$
4: 将比较数组$C_{j}$和$C_{m}$设置为{-1}
5: 并行遍历 $j\in[0,N_{g})$ ⊲ 执行条件比较
6: $C_{m}[i] \leftarrow$ 比较$i$与$j$的$G_{\nu}[i]$和$G_{\nu}[j]$
7: $C_{j}[i] \leftarrow$ 进一步比较$G_{i}[i]$和$G_{i}[j]$
8: 结束for循环。
9: 如果 $C_{j}[i] = 0$ 且 GP[i].r < Q_{j}.r

---

以上翻译已根据专家建议进行了修正，确保准确性、流畅性、风格一致性和术语一致性。<}Q_{l}$ .r & $G_{\nu}[\mathrm{i}]{=}0$ & $G_{i}[\mathrm{i}]\!>\!0$ then   
10: $Q_{l}\leftarrow\mathrm{GP[i]}$   
11: end if   
12: if $C_{m}[\mathrm{i}]{=}0$ & GP[i]. $\mathbf{r}{<}Q_{m}$ .r & $0{<}G_{\nu}[\mathrm{i}]{<}3$ then   
13: $Q_{m}\leftarrow\mathrm{GP[i]}$   
14: end if   
15: end for  

information and the distance-ordered candidate points, as detailed in Algorithm 2.  

The methodology for PNN-C can be adapted for other correspondence types:  

• For KNN-C: Skip step 5 and directly output KNN correspondences in step 4.   
• For ENN-C: Modify step 5 to identify $Q_{\mathcal{E}}^{m}$ and divide the column index $h$ by 5 to account for the reduced size of the point cloud matrix in the specialized ENN-C RPS structure.  

# B. Hardware Implementation of RPS-Searcher  

This subsection outlines the hardware architecture and optimization strategies of the proposed RPS-Searcher accelerator, which enhances task-level pipeline performance while maintaining high search accuracy. Based on the search algorithm in Section V-A, the accelerator comprises seven modules: Transform-Points-Projection (TPP), Compute-SearchRegion (CSR), Extract-Region-Index (ERI), Extract-NearPoints (ENP), Points-Batcher (PB), KNN-CS, and PNN-CS, as shown in Fig. 6. Key optimizations for each module are detailed below.  

1) TPP Module: The TPP module transforms source points into the target coordinate system and projects query points into the RPS structure. Two primary optimizations are applied:  

• Bit-width Optimization: The bit-width of source points is optimized to 20 bits, ensuring suffcient accuracy while minimizing hardware resources.   
• Matrix Multiplication Unrolling: The matrix multiplication process is fully unrolled to accelerate computations and improve hardware effciency.  

2) CSR Module: The CSR module computes search regions for query points, represented by column indices. As illustrated in Fig. 10, a proximity-based search optimization strategy is employed, using a sequence generator $(0,-1,1,-2,2,\ldots)$ to prioritize nearby columns. Once suffcient points are found, searches in distant columns are skipped, improving effciency.  

![](images/28ef04b45ba6fce25b43942121898fb259f49d25ec638a2d5fb54d5f50b0055e.jpg)  
Fig. 10. Hardware Implementation of the CSR and ERI module.  

3) ERI Module: The ERI module extracts search blocks $(S B_{m i n},S B_{m a x})$ from search regions. There are two challenges when implementing the ERI module. (1) The capacity of onchip memory is insuffcient to store the entire RPS-Index array, and accessing external memory introduces signifcant latency. (2) Redundant points within the search region negatively impact performance. To address these challenges, as Fig. 10 shows, we present a dynamic caching strategy that incorporates the following three techniques:  

• Optimized on-chip caching: Observing that both the RPS-Index and input query points are roughly ordered in a RSSD-wise manner, we design an on-chip sliding window cache, referred to as the RPSI-Cache (Fig. 6). The RPSI-Cache has two key features: (1) It is compact, occupying only about $3.5\%$ of the size of the original RPS-Index array. This reduced size is suffcient to overlap with neighboring search regions for any query point, as confrmed by profling results. (2) The RPSI-Cache is dynamically refreshed based on the location of the query point, ensuring continuous coverage of adjacent regions. • Effcient cache refresh strategy: We refresh the outdated portion of the RPSI-Cache with new data when the column index of the query point exceeds the current reading column index by a predefned threshold. This strategy is crucial for maintaining the high-throughput pipeline structure of the ERI module. To further improve performance, the RPSI-Cache is cyclically partitioned into multiple segments and bound to 2-Port BRAM, enabling concurrent reading and writing of multiple RPSI values simultaneously.  

4) ENP Module: The ENP module is designed to extract candidate corresponding points from the RPS-Points array based on the identifed search blocks. To enhance effciency, we also adopt the dynamic caching strategy while further optimizing points-read parallelism to meet performance requirements. We pack 4 candidate points into a single large bit-width element in the RPSP-Cache array and partition the array into 16 independent segments, enabling the concurrent reading of all 48 candidate points in parallel.  

5) PB Module: The PB module aggregates a variable number of candidate points into fxed-size batches to enable effcient dense computation in the subsequent KNN-CS module.  

The motivation for this design lies in the highly variable number of candidate points for a single input, which can range from 1 to 48. Directly transmitting up to 48 points to the next stage would require padding most of the time, introducing ineffciencies such as wasted hardware resources, increased power consumption, and elongated clock paths, which degrade timing performance. At the same time, discarding excess points would compromise search accuracy, as critical data might be lost.  

To address this, we propose a dynamic batching strategy that adjusts the varying numbers of candidate points in the RSSD and packages them into fxed-size batches compatible with the requirements of the next module. By dynamically controlling data buffering and output, the PB module ensures that the search accuracy is preserved, while optimizing resource utilization and reducing overhead.  

Effcient buffering with a shift batch buffer is employed to implement this module in a pipeline architecture while minimizing hardware resource consumption. The buffer, sized at $256\!\times\!64$ , is controlled by a pointer that tracks the number of valid candidate points. Incoming data is stored at the location indicated by the pointer, while invalid points are overwritten by new valid points, ensuring effcient memory use. Once the pointer exceeds a predefned threshold, the module outputs a dense batch of points to the next stage and resets the pointer. This design not only ensures high throughput but also minimizes resource usage.  

6) KNN-CS Module: The KNN-CS module focuses on sorting candidate corresponding points based on their distance to the query point and selecting the K-Nearest Neighbors (KNN) to identify $Q_{\mathcal{H}}^{j}$ .  

Sorting and selecting KNN points from streaming candidate points is computationally expensive due to the large number of required comparisons and excessive data bandwidth consumption. A naive sorting approach would require a large comparator array, increasing hardware complexity and power consumption.  

To address this, we employ an optimized K-selection priority queue method, which effciently selects and sorts the KNN points while reducing the number of comparators and narrowing data bandwidth. As candidate points stream into the module, a parallel comparing network is used to select KNN points from the incoming data, signifcantly reducing the data bandwidth. These KNN points are then compared with the contents of a priority queue, which is also sized to K, to update the queue. Once all candidate points have been processed, the priority queue contains the fnal KNN correspondences.  

This approach leverages an effcient priority queue architecture introduced in our prior work [30], allowing for reduced hardware resource usage and lower power consumption, while simultaneously improving the performance of subsequent stages.  

7) PENN-CS Module: The PENN-CS module is responsible for selecting PNN-C or ENN-C from the candidate geometric points. The selection process is computationally intensive, as it typically requires large comparator arrays and resource-heavy in-order comparisons to fnd the desired points. Moreover, unordered geometric points make the selection process less effcient, increasing resource utilization and processing time.  

To optimize this process, we leverage laser channel information and the ordered nature of KNN points to improve effciency and reduce hardware requirements. The module employs two key strategies:  

• Effcient point selection using laser channel attributes: Unlike previous methods [30] that rely on large comparator arrays, we use the unique laser channel properties of points within a search block for direct selection. Points in a search block belong to the same column and have unique laser channel indices (row index), and the laser channel of $Q_{\mathcal{H}}^{m}$ matches that of $Q_{\mathcal{H}}^{j}$ . Based on this observation, we directly compare laser channels and distances to rapidly identify $Q_{\mathcal{H}}^{m}$ , as detailed in Algorithm 2. This signifcantly reduces the comparator array size while improving selection accuracy.   
• Resource-effcient compare logic optimization: Instead of selecting from unordered points, the PENN-CS module identifes $Q_{\mathcal{H}}^{m}$ from the already ordered KNN points. First, the most suitable point $\widehat{Q}_{\mathcal{H}}^{l}$ is selected from the input K candidate geometric po ints based on their laser channels and order indices. Then, $\widehat{Q}_{\mathcal{H}}^{l}$ is compared with the current $\boldsymbol{Q}_{\boldsymbol{H}}^{l},$ and the nearer poin t  is retained in $Q_{\mathcal{H}}^{l}$ . This reduces both the number and bit-width of required comparators, signifcantly lowering resource utilization and improving effciency.  

By combining laser channel-based selection and compare logic optimization, the PENN-CS module achieves a balance between performance and resource effciency, ensuring that geometric corresponding points are selected accurately with minimal hardware overhead.  

# VI. EXPERIMENTAL RESULTS AND ANALYSIS  

In this section, we frst describe the experimental setup, encompassing the test datasets, evaluation cases, parameters, various implementations, and testing platforms. Subsequently, we conduct a comparative analysis of runtime, power consumption, and resource utilization across different correspondence search implementations. Following this, we provide a detailed analysis of the proposed RPS-CS accelerator, including the latency and resource utilization of individual modules. Finally, we integrate the RPS-CS based registration framework into the L-SLAM system, replacing its existing registration algorithm. This integration enables us to evaluate the framework’s impact on accuracy and runtime performance in a practical application context.  

# A. Experimental Setup  

Experimental datasets: To evaluate the proposed point cloud RPS-CS accelerator and registration framework, we utilize the KITTI dataset [10], specifcally the Visual Odometry/SLAM Evaluation 2012 point cloud. This dataset includes diverse road environment data captured during real-world driving scenarios. For our evaluation, we focus exclusively on point clouds collected by the Velodyne HDL-64E LiDAR sensor, which employs 64 lasers and operates at a scanning frequency of $10\mathrm{Hz}$ .  

TABLE I EXPERIMENTAL TEST CASES.   


<html><body><table><tr><td>Cases</td><td>Case I</td><td>Case II</td><td>Case IⅢI</td><td colspan="2">Case IV</td></tr><tr><td>Type</td><td>All Types</td><td>All types</td><td>All t types</td><td>PNN-C</td><td>ENN-C</td></tr><tr><td>NQ</td><td>30,000</td><td>1,000 to 80,000</td><td>30,000</td><td>30,000</td><td>5000</td></tr><tr><td>Np</td><td>30,000</td><td>30,000</td><td>1,000 to 80,000</td><td>1,400</td><td>750</td></tr></table></body></html>实验案例：为了全面评估所提出的RPS-CS加速器的性能，我们设计了四个具有代表性的实验案例，如表一所示。案例I采用相关文献[5]、[14]中的标准设置，作为对比基准。案例II和案例III评估了在不同$N_{P}$和$N_{Q}$配置下加速器的可扩展性和鲁棒性。最后，案例IV通过分析典型点云配准任务中$N_{P}$和$N_{Q}$的平均大小，评估了RPS-CS加速器的实际应用能力，如[1]所述。

对于所有案例，点云数据通过对KITTI数据集中不同场景下随机选取的十个连续帧的点云进行降采样获得。

实验平台：ARM平台的测量是在配备Apple Silicon M4的Mac Mini（4.4 GHz）上进行的。FPGA实现是通过Xilinx Vitis 2022.2开发的，并在Xilinx UltraScale+ MPSoC ZCU104开发板（200 MHz）上进行了验证。FPGA平台的功耗测量通过嵌入式INA226传感器经由PMBus获取；ARM平台的测量则使用官方电源管理库API。所有测量均反映整个平台的总功耗，以确保公平性比较。

实验参数：根据KITTI数据集中使用的64线激光雷达（64-laser LiDAR）的规格，我们严格配置了实验参数。具体来说，我们……将水平分辨率 $H=1800$（ENN-C 为 360）和垂直分辨率 $V=64$ 设置以便于生成点云数据矩阵。此外，我们设置 $N_{sd}=4$，$M=72$，以及 $r_{in}=1$。这确保对应搜索结果的准确性在所有测试案例中均超过95%。

# B. 运行时间、功耗和资源的比较

表 II 总结了各种对应搜索方法在测试案例 I 中每帧的平均运行时间。基于 FPGA 的 PNN-CS 时间是通过结合高并行度 $\boldsymbol{\mathrm{k}}$-选择实现 [5] 和报告的 KNN-CS 时间得出的。对原始 PNN 操作 [1] 的分析表明，它平均搜索 2,338 个邻近点以确定每个查询点的 PNN 对应关系。为了解决这个问题，我们开发了一个高并行度 $\boldsymbol{\mathrm{k}}$-选择加速器，能够在 $21.9~\mathrm{ms}$ 内处理 30,000 个查询的 2,338 个邻近点。由于 k-选择加速器与原始 KNN-CS 加速器在任务层级流水线中并行工作，因此基于 FPGA 的 PNN-CS 时间取 KNN-CS 时间和 $21.9~\mathrm{ms}$ 的最大值作为最终时间。![](images/e7aa1344db87d091548fd38d933d9678ad71d8953d247043653746ef75821d40.jpg)图11. RPS-CS加速器的性能结果。

结果表明，所提出的RPS-CS加速器实现了卓越的性能，仅需$5.0~\mathrm{ms}$即可为30,000个点构建数据结构并搜索PNN对应关系。与其它实现相比，我们的RPS-CS加速器在FPGA平台上分别比QuickNN [14]快13.6倍，比ParallelNN [9]快7.5倍（基于迭代次数），比RPS-KNN [31]快4.7倍。此外，RPS-CS加速器在数据结构构建时间和KNN/PNN对应关系搜索方面均达到最优性能，证实了RPS结构的高效性以及RPSBuilder和RPS-Searcher加速器的高度并行性。

表II还比较了不同实现的平均功耗和能量消耗。以FLANN-CPU为基准，能量效率比（EER）定义为基准能量与某实现能量消耗的比率。RPS-CS加速器的能量效率分别是QuickNN [14]的20.7倍、ParallelNN [9]的187倍和RPS-KNN [31]的3.6倍。这种能量效率主要归功于动态电压频率调节（DVFS）[32], [33]，其中可编程逻辑（PL）侧的供电电压在固定的${200}\,\mathrm{MHz}$频率下动态调节，以最小化功耗并保持正确结果。

由于性能（$5.0\,\mathrm{ms}$）远超实时要求，我们专注于其他优化方向。我们在面积优化方面进一步提高了能效。表III显示了RPS-Builder、RPS-Searcher以及总RPS-CS加速器的完全布线资源利用率。与QuickNN相比，RPS-CS使用的可重构资源（查找表和触发器）及DSP更少，但需要更多的内存来存储RPS-Points和RPS-Index数组。同样，除了查找表之外，RPS-CS使用的硬件资源比DSVS少。

# C. RPS-CS加速器的详细分析

为了全面分析所提出的RPS-CS加速器，我们评估了其在测试案例II和III中的性能。图11显示了RPS结构构建和对应关系搜索的运行时间。

实验结果表明，RPS结构的构建时间对于KNN/PNN对应关系始终固定为1.4 毫秒，对于SNN对应关系为0.4 毫秒。这种一致性是由于点云矩阵的组织方式固定（对于KNN/PNN，V为64、H为1800；对于SNN，V为64、H为450）。构建模块基于此固定大小处理所有操作，确保了构建时间的恒定性。

表II 不同KNN/PNN对应关系搜索实现的比较<html><body><table><tr><td>Implementations</td><td>TPAMI'14 (FLANN) [4]</td><td>Trans.IE'20 (Graph) [15]</td><td>HPCA'20 (QuickNN) [14]</td><td>TCAS-II'20 (DSVS)[5]</td><td>HPCA'23 (ParallelNN) [9]</td><td>ISCAS'23 (RPS-KNN) [31]</td><td>This Work (RPS-CS)</td></tr><tr><td>Hardware</td><td>CPU (Mac Mini M4)</td><td>FPGA (ZCU102)</td><td>FPGA (VCU118)</td><td>FPGA (ZCU102)</td><td>FPGA (VCU128)</td><td>FPGA (ZCU104)</td><td>FPGA (ZCU104)</td></tr><tr><td>Process</td><td>3nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td><td>16nm</td></tr><tr><td>SearchStructure</td><td>K-D Tree</td><td>Graph</td><td>K-D Tree</td><td>DSVS</td><td>Octree (1024)</td><td>RPS</td><td>RPS</td></tr><tr><td>Build Time (ms)</td><td>3.4</td><td>289.0</td><td>46.0</td><td>4.8</td><td>0.5</td><td>1.5</td><td>1.4</td></tr><tr><td>KNN-CS Time (ms)</td><td>9.0</td><td>146.0</td><td>10.5</td><td>69.0</td><td>2.0</td><td>2.6</td><td>2.8</td></tr><tr><td>PNN-CS Time (ms)</td><td>110.1</td><td>146.0</td><td>21.9</td><td>69.0</td><td>21.9</td><td>21.9</td><td>3.6</td></tr><tr><td>Total Time (ms) +</td><td>113.5</td><td>435.0</td><td>67.9</td><td>73.8</td><td>22.4</td><td>23.4</td><td>5.0</td></tr><tr><td>Speedup Ratio +</td><td>1.0</td><td>0.3</td><td>1.7</td><td>1.5</td><td>3.0</td><td>4.9</td><td>22.7</td></tr><tr><td>Average Power (W)</td><td>12.9</td><td>4.2</td><td>4.7</td><td>3.6</td><td>28.4</td><td>2.4</td><td>3.1</td></tr><tr><td>Energy (mJ)</td><td>1464.2</td><td>1827.0</td><td>321.2</td><td>265.7</td><td>636.2</td><td>55.2</td><td>15.5</td></tr><tr><td>EER +</td><td>1.0</td><td>0.8</td><td>4.6</td><td>5.5</td><td>0.5</td><td>26.5</td><td>94.5</td></tr></table></body></html>

\* PNN-CS time is based on a high-parallel $\mathbf{k}$ -selection implementation [5] and the reported KNN-CS time. + Total Time is the sum of Build Time and PNN-CS Time. Speedup Ratio is the Total Time relative to the baseline. EER (Energy-Effciency Ratio) is the Energy relative to the baseline.  

TABLE III RESOURCE UTILIZATION COMPARISON OF DIFFERENT CORRESPONDENCE SEARCH ACCELERATORS   


<html><body><table><tr><td colspan="2">Implmentations</td><td>LUTs</td><td>FFs</td><td>BRAM36</td><td>DSP</td></tr><tr><td>QuickNN [14]</td><td>Total</td><td>203,758</td><td>152,962</td><td>1</td><td>896</td></tr><tr><td>DSVS [5]</td><td>Search</td><td>68,960</td><td>65,024</td><td>350</td><td>83</td></tr><tr><td>ParalleINN [9]</td><td>Search</td><td>141,132</td><td>177,112</td><td>116</td><td>480</td></tr><tr><td rowspan="3">RPS-CS (ours)</td><td>Build</td><td>56,733</td><td>43,372</td><td>105</td><td>19</td></tr><tr><td>Search</td><td>93,149</td><td>52,465</td><td>251</td><td>43</td></tr><tr><td>Total</td><td>153,722</td><td>143,765</td><td>362.5</td><td>62</td></tr></table></body></html>搜索时间与源点数量呈现出近乎线性的关系，表明RPS缓存[31]中的性能瓶颈已解决。这一改进源于扩大缓存端口位宽并优化缓存策略以动态适应查询需求。然而，当查询数量低于3,000时，由于大量目标点的数据传输时间占主导，PNN-C和KNN-C的搜索时间保持不变。超过这一阈值后，搜索时间成为主要因素，随着查询数量的增加而线性增长。

此外，这种线性增长的斜率由预设的最大搜索范围决定。由于KNN-C和ENNC采用相同的搜索范围，它们的斜率几乎相同。相比之下，为了保持95%以上的准确率，PNN-C需要更大的搜索范围，因此其斜率更陡，随着查询数量的增加，搜索时间增长更快。

# D. 配准框架评估

为了验证RPS-CS加速器在配准框架中的性能表现，我们在测试案例IV上评估其运行时性能，并评估其集成到SLAM系统后对准确性和速度的影响。

表IV 不同注册实现的准确率比较表<html><body><table><tr><td rowspan="2">Seq.</td><td rowspan="2">Enviroment</td><td colspan="2">LOAM</td><td colspan="2">LOAM-RPS</td><td colspan="2">LOAM-RPS-HW</td></tr><tr><td>trel</td><td>rrel</td><td>trel</td><td>rrel</td><td>trel</td><td>rrel</td></tr><tr><td>00</td><td>Urban</td><td>1.0340</td><td>0.0046</td><td>1.0369</td><td>0.0047</td><td>1.0844</td><td>0.0047</td></tr><tr><td>01</td><td>Highway</td><td>2.8464</td><td>0.0060</td><td>2.8332</td><td>0.0060</td><td>2.8357</td><td>0.0060</td></tr><tr><td>02</td><td>Urban+Country</td><td>3.0655</td><td>0.0114</td><td>3.1339</td><td>0.0114</td><td>3.2206</td><td>0.0117</td></tr><tr><td>03</td><td>Country</td><td>1.0822</td><td>0.0066</td><td>1.0858</td><td>0.0066</td><td>1.0884</td><td>0.0066</td></tr><tr><td>04</td><td>Country</td><td>1.5248</td><td>0.0055</td><td>1.5336</td><td>0.0054</td><td>1.5364</td><td>0.0054</td></tr><tr><td>05</td><td>Urban</td><td>0.7184</td><td>0.0036</td><td>0.7520</td><td>0.0037</td><td>0.7705</td><td>0.0038</td></tr><tr><td>06</td><td>Urban</td><td>0.7627</td><td>0.0040</td><td>0.7781</td><td>0.0040</td><td>0.7775</td><td>0.0041</td></tr><tr><td>07</td><td>Urban</td><td>0.5629</td><td>0.0040</td><td>0.5719</td><td>0.0038</td><td>0.5750</td><td>0.0038</td></tr><tr><td>08</td><td>Urban+Country</td><td>1.2320</td><td>0.0052</td><td>1.1680</td><td>0.0048</td><td>1.1659</td><td>0.0049</td></tr><tr><td>09</td><td>Urban+Country</td><td>1.3103</td><td>0.0053</td><td>1.3062</td><td>0.0052</td><td>1.3041</td><td>0.0053</td></tr><tr><td>10</td><td>Urban+Country</td><td>1.6843</td><td>0.0059</td><td>1.6317</td><td>0.0059</td><td>1.6211</td><td>0.0060</td></tr><tr><td></td><td>MeanError</td><td>1.4385</td><td>0.0056</td><td>1.4392</td><td>0.0056</td><td>1.4527</td><td>0.0057</td></tr></table></body></html>$t_{rel}$：在100米至800米长度范围上的平均平移RMSE（%）。$r_{rel}$：在100米至800米长度范围上的平均旋转RMSE（每100米的旋转RMSE，度）。

在案例IV中，RPS-CS加速器实现了与图11所示的运行时间一致。对于SNN-C，构建RPS结构耗时0.4毫秒，搜索对应点匹配耗时0.6毫秒。对于PNN和KNN对应点匹配，构建耗时1.4毫秒，搜索耗时1.9毫秒。

为了评估精度，将RPS-CS加速器集成到一个SLAM系统中，并通过以下方案评估定位轨迹精度：

• LOAM：原始LOAM [1]，在FPGA PS（Processing System）侧实现，作为基准。
• LOAM-RPS：带有浮点RPS-CS集成的LOAM，在FPGA PS侧实现。
• LOAM-RPS-HW：带有定点RPS注册框架的LOAM，在FPGA PL（Programmable Logic）侧实现。

三种方案的精度结果如表IV所示，使用官方KITTI评估工具进行评估。可以得出结论，我们提出的基于RPS的注册框架具有几乎可以忽略不计的精度损失，观测到的精度下降约为1%。在某些场景下，由于减少了异常值对应点对，我们的框架甚至能实现更高的定位精度。<html><body><table><tr><td colspan="5">LOAM</td></tr><tr><td>75.6</td><td></td><td>142</td><td>246.6</td><td>42.8</td></tr><tr><td>LOAM-RPS</td><td></td><td></td><td></td><td>507ms</td></tr><tr><td>68.5</td><td>51</td><td>93.5</td><td>42.8 255.8ms</td><td></td></tr><tr><td>LOAM-RPS-HW</td><td></td><td></td><td></td><td></td></tr><tr><td>42.8</td><td>49.6ms</td><td></td><td></td><td></td></tr><tr><td></td><td></td><td></td><td></td><td></td></tr><tr><td>1.8 1.2 3.8</td><td></td><td></td><td></td><td></td></tr></table></body></html>图12展示了KITTI数据集上的平均点云配准时间。在点云配准任务中进行两次对应关系搜索迭代后，SNN和PNN的搜索时间分别达到1.2 ms和3.8 ms。基于RPS结构的配准算法实现了整体速度的2倍提升，对应关系搜索时间提升至原速度的2.7倍。使用所提出的框架，整体运行速度提高了10.2倍，达到20.1 FPS（每秒帧数），而对应关系搜索加速了68.2倍。

# 第七章 结论

在这项工作中，我们提出了一种实时的基于FPGA的点云配准框架，具有可配置且超快速的对应关系搜索能力。首先，我们开发了一种新颖的Range-Projection Structure（RPS结构），它将无结构的LiDAR点组织成类似矩阵的结构，以实现点云的高效定位，并将附近的点分组到连续的内存段中以提升点云数据的访问效率。其次，我们引入了一种高效的多模式对应关系搜索算法，该算法利用RPS结构有效缩小搜索区域，消除冗余点，并通过利用激光雷达特有的激光通道信息支持各种类型的对应关系。第三，我们设计了一个可配置的超快速基于RPS结构的对应关系搜索（RPS-CS）加速器，其特点是高性能的RPS-Builder用于快速结构构建，以及高度并行的RPS-Searcher用于快速对应关系搜索，进一步提升运算效率。通过动态缓存策略和流水线批处理模块来提升效率和可配置性。实验结果表明，RPS-CS加速器相比最先进的FPGA实现实现了7.5倍的速度提升和17.1倍的能效改进，而所提出的框架对64线激光雷达数据达到了20.1 FPS的实时性能（每秒20.1帧）。

在未来的工作中，我们将致力于克服大规模并行化的带宽限制，并扩展所提出的方法以适应通用型点云。

# 参考文献

[1] J. Zhang 和 S. Singh, “低漂移和实时激光雷达测程与建图,” 自主机器人, 第41卷，第2期，第401–416页，2017年2月。
[2] X. Zhang 和 X. Huang, “激光雷达点云的实时快速通道聚类,” IEEE电路与系统汇刊II: 快报, 第69卷，第10期，第4103–4107页，2022年10月。
[3] C.-C. Wang, Y.-C. Ding, C.-T. Chiu, C.-T. Huang, Y.-Y. Cheng, S.-Y. Sun, C.-H. Cheng 和 H.-K. Kuo, “用于手势分类的实时基于块的嵌入式CNN在FPGA上的实现,” IEEE电路与系统汇刊I: 正规论文, 第68卷，第10期，第4182–4193页，2021年10月。
[4] M. Muja 和 D. G. Lowe, “高维数据的可扩展最近邻算法,” IEEE模式分析与机器智能汇刊, 第36卷，第11期，第2227–2240页，2014年11月。
[5] H. Sun, X. Liu, Q. Deng, W. Jiang, S. Luo 和 Y. Ha, “高效的FPGA实现”（原文不完整，请检查或补充）。k-最近邻搜索算法的3D激光雷达定位与建图，《IEEE电路与系统汇刊II：特快简报》，第67卷，第9期，第1644-1648页，2020年9月。

[6] 邓琪、孙浩、陈飞、舒宇、王辉、韩一，《一种基于FPGA的实时优化NDT（正态分布变换）算法用于智能车辆3D激光雷达定位》，《IEEE电路与系统汇刊II：特快简报》，第68卷，第9期，第3167-3171页，2021年9月。

[7] E. Bank Tavakoli、A. Beygi和X. Yao，《RPKNN：一种基于OpenCL的FPGA实现降维KNN算法，基于随机投影》，《IEEE非常大规模集成（VLSI）系统汇刊》，第30卷，第4期，第549-552页，2022年4月。

[8] 王超、黄志、任安、张晓，《一种基于FPGA的KNN搜索加速器，用于点云配准》，见《2024年IEEE国际电路与系统研讨会（ISCAS）论文集》，2024年5月，第1-5页。

[9] 陈飞、英睿、薛静、文方、刘鹏，《《ParallelNN：一种基于八叉树的并行最近邻搜索加速器，用于3D点云》》，见《2023年IEEE高性能计算机体系结构国际研讨会（HPCA）论文集》，2023年2月，第403-414页。

[10] 韩敏、王磊、肖亮、张华、蔡涛、徐俊、吴颖、张晨、徐翔，《BitNN：一种位串行K最近邻搜索加速器，用于点云》，见《2024年ACM/IEEE第51届国际计算机体系结构研讨会（ISCA）论文集》，2024年6月，第1278-1292页。

[11] F. Groh、L. Ruppert、Peter Wieschollek，《一种基于FPGA的高效点云处理方法》，见《2024年IEEE国际电路与系统研讨会（ISCAS）论文集》，2024年5月。Hollek 和 H. P. A. Lensch， “Ggnn：基于图的GPU最近邻搜索”，《IEEE大数据学报》：第9卷，第1期，267–279页，2023年2月。
[12] K. Koide， M. Yokozuka， S. Oishi 和 A. Banno，“体素化-GICP用于快速和精确的3D点云配准”，在2021年IEEE国际机器人与自动化会议（ICRA），5月，11054–11059页。
[13] W. Dong， J. Park， Y. Yang 和 M. Kaess，“GPU加速的鲁棒性场景重建”，在2019年IEEE/RSJ智能机器人与系统国际会议（IROS），11月，7863–7870页。
[14] R. Pinkham， S. Zeng 和 Z. Zhang，“QuickNN：基于K-D树的3D点云最近邻搜索的内存及性能优化”，在2020年IEEE高性能计算机体系结构国际研讨会（HPCA），2月，180–192页。
[15] A. Kosuge， K. Yamamoto， Y. Akamine 和 T. Oshima，“基于SOC-FPGA的迭代最近点加速器，以实现更快的拣选机器人”，《IEEE工业电子学报》：第68卷，第4期，3567–3576页，4月2021年。
[16] T. Shan 和 B. Englot，“Lego-LOAM：轻量级且地面优化的激光雷达里程计和地图构建，适用于可变地形”，在2018年IEEE/RSJ智能机器人与系统国际会议（IROS），10月，4758–4765页。
[17] H. Wang， C. Wang， C.-L. Chen 和 L. Xie，“F-LOAM：快速激光雷达里程计和地图构建”，在2021年IEEE/RSJ智能机器人与系统国际会议（IROS），9月，……4390–4396。   
[18] 张勇，周志，P. David，X. Yue，Z. Xi，龚斌和H. Foroosh，“Polarnet：一种改进的在线激光雷达点云语义分割网格表示方法”，见《IEEE/CVF计算机视觉与模式识别会议论文集》(CVPR)，2020年6月，第9601至9610页。   
[19] 孙睿，钱杰，R.H. Jose，龚振，苗锐，薛伟和刘鹏，“一种灵活高效的基于ORB的实时全高清图像特征提取加速器”，《IEEE超大规模集成电路（VLSI）系统汇刊》，第28卷，第2期，第565至575页，2020年2月。   
[20] 刘瑞，杨健，陈宇，赵文，“ESLAM：一种用于FPGA平台上的实时ORB-SLAM的节能加速器”，见《2019年第56届ACM/IEEE设计自动化会议论文集》(DAC)，2019年6月，第1至6页。   
[21] 杨浩，石军和L. Carlone，“TEASER：快速且可验证的点云配准”，《IEEE机器人学汇刊》，第37卷，第2期，第314至333页，2021年4月。   
[22] 马飞，G.V. Cavalheiro和S. Karaman，“自监督稀疏到密集：来自激光雷达和单目相机的自监督深度填充”，见《2019年国际机器人与自动化会议论文集》(ICRA)，2019年5月，第3288至3295页。   
[23] 吕毅，白亮和黄晓龙，“Chipnet：FPGA上用于可行驶区域划分的实时激光雷达处理”，《IEEE电路与系统汇刊I：常规论文》，第66卷，第I卷，第5期，第1769至1779页，2019年5月。   
[24] 刘洋，李静，黄凯，李翔，Qi Xin，龙宇和周建，“Mobilesp：一种用于移动VSLAM的基于FPGA的实时关键点提取硬件加速器，”《IEEE电路与系统汇刊I：常规论文》，第69卷，第12期，第4919—4929页，2022年12月。

[25] 张鑫、张磊、娄晓旭，“基于原始图像的端到端目标检测加速器，采用HOG特征，”《IEEE电路与系统汇刊I：常规论文》，第69卷，第1期，第322—333页，2022年1月。

[26] 李勇、李明、陈超、邹翔、邵辉、唐飞和李凯，“Simdiff：利用空间相似性和差异执行的点云处理加速，”《IEEE计算机辅助设计集成电路与系统汇刊》，第44卷，第2期，第568—581页，2025年2月。

[27] 高阳、江晨、皮亚德、陈曦、帕特尔和林浩，“HGPCN：一种用于端到端嵌入式点云推理的异构架构，”在2024年第57届IEEE/ACM国际微体系结构研讨会（MICRO），2024年11月，第1588—1600页。

[28] 严刚、刘星、陈峰、王浩和哈英，“具有脉冲推送和早期终止的图割算法的超快速FPGA实现，”《IEEE电路与系统汇刊I：常规论文》，第69卷，第4期，第1532—1545页，2022年4月。

[29] 陈超、邹翔、邵辉、李勇和李凯，“通过利用几何相似性进行点云处理加速的方法，”在2023年第56届IEEE/ACM国际微体系结构研讨会（MICRO），2023年12月，第1135—1147页。

[30] 孙浩、邓强、刘星、舒宇和哈英，“一种节能的流式FPGA（原文为‘energy-effcient’，应为‘节能’）”激光雷达点云有效局部搜索特征提取算法的实现,”《IEEE电路与系统汇刊I：常规论文集》，第70卷，第1期，第253—265页，2023年1月。

[31] 肖杰、孙浩、邓强、刘翔、张华、何超、舒颖、哈勇，“Rps-kNN：智能车辆激光雷达里程计的超快速FPGA加速器——基于范围投影结构的k近邻搜索”，在2023年IEEE国际电路与系统学术会议（ISCAS），2023年5月，第1—5页。

[32] 陈飞、余浩、江伟、哈勇，“基于深度强化学习的能量采集边缘设备自适应应用质量优化方法”，《IEEE计算机辅助设计集成电路与系统汇刊》，第41卷，第11期，第4873—4886页，2022年11月。

[33] 江伟、余浩、张华、舒颖、李睿、陈佳、哈勇，“Fodm：支持FPGA上所有时序路径的精确在线时延测量框架”，《IEEE超大规模集成电路（VLSI）系统汇刊》，第30卷，第4期，第502—514页，2022年4月。![](images/114adf66878a944931bb625d0ee0e6d43293f9573dbb5fb05e9ddf211e0d5c2c.jpg)  

Qi Deng received the B.S.degree in electronic and information engineering from ShanghaiTech University in 2018. He is currently pursuing the Ph.D degree with ShanghaiTech University; the Shanghai Advanced Research Institute Chinese Academy of Sciences: and the University of Chinese Academy of Sciences. His research interests include localization and perception algorithms in smart vehicles and its hardware acceleration.  

![](images/b6c2bb377630983a6da333dee552abf6bf2c0183fef628ffe3a9b0f9b343e422.jpg)孙浩（S’20–M’23）于2018年获得东南大学学士学位。他于2023年在中国科学院上海微系统与信息技术研究所与上海科技大学可重构与智能计算实验室联合授予的博士学位。目前，他在东南大学担任讲师。他的研究兴趣主要集中在定制计算、硬件加速、基于激光雷达的定位与地图构建。

肖建中于2020年获得南京航空航天大学自动化学院自动化专业学士学位。目前，他正在中国上海的上海科技大学攻读博士学位。他的研究兴趣包括硬件加速、超低功耗VLSI（超大规模集成电路）设计和定位。![](images/f844cd5ac55c56f0cdff24eab058b266aa0a8d2da4d47fc4f6136181784bef4f.jpg)  

![](images/98455084f18086a0f53a17ee238725a884f630bbf6bc01a116db19b012905bfe.jpg)  

Yuhao Shu (S’21) received the B.S. degree in electronic science and technology from Hefei University of Technology, Hefei, China, in 2019. He received the Ph.D. degree in electronic science and technology from ShanghaiTech University, Shanghai, China, in 2025. Currently, he is working as an associate professor at Nanjing University of Aeronautics and Astronautics, Nanjing, China. His current research interests include embedded memory design, in-memory computing, cryogenic CMOS circuits, and ultra-low power VLSI design.  

![](images/2ccf403b0c4e61cc006bea4c1020c8a4661755602ca3a99f548464c7c6ebbd18.jpg)姜伟雄于2017年在哈尔滨工业大学获得学士学位。他于2022年在中国科学院上海微系统与信息技术研究所（上海）和上海科技大学可重构智能计算实验室（上海）获得工学博士学位。他的主要研究方向包括高效的DNN加速以及FPGA上的在线时序余量测量。

王辉于2001年在中国科学院半导体研究所获得物理学博士学位。他是中国科学院上海高等研究院的正教授，从事微电子学研究。他的主要研究方向包括高性能成像和显示面板驱动技术。![](images/98d3875b42db1133cc2cf49ea74ada3688fbfc92b5077f04d696130293f645dc.jpg)  

![](images/b0d31b674bdf5953389fd09a4b44db8c6e898929a82b8e125f0ab8ee6508c22a.jpg)哈亚军（S’98–M’04–SM’09）于1996年在中国杭州的浙江大学获得学士学位，1999年在新加坡国立大学获得工程硕士学位，2004年在比利时鲁汶的天主教鲁汶大学获得电气工程博士学位。

他目前担任中国上海科技大学的教授。在此之前，他是新加坡资讯通信研究院（I2R）-比亚迪联合实验室的科学家兼主任，以及新加坡国立大学电气与计算机工程系的兼任副教授（兼职）。更早之前，他是新加坡国立大学的助理教授。

他的研究兴趣包括可重构计算、超低功耗数字电路和系统、嵌入式系统架构及其设计工具，并应用于机器人、智能车辆和智能系统。他在这些领域发表了大约150篇国际同行评审的期刊和会议论文。

他在学术界担任过多个职位。他曾任IEEE《电路与系统汇刊II：特快简报》（2022-2023年）主编，IEEE《电路与系统汇刊II：特快简报》（2020-2021年）副主编，IEEE《电路与系统汇刊I：正刊论文》（2016-2019年）副主编，IEEE《电路与系统汇刊II：特快简报》（2011-2013年）副主编，IEEE《超大规模集成电路（VLSI）系统汇刊》（2013-2014年）副主编，自2009年起担任《低功耗电子学杂志》编委。他曾担任ISICAS 2020的技术程序委员会联合主席，ASP-DAC 2014的大会共同主席；FPT 2010和FPT 2013的程序委员会共同主席；IEEE电路与系统学会新加坡分会主席（2011年和2012年）；ASP-DAC指导委员会成员；IEEE电路与系统学会VLSI与应用技术委员会成员。他还曾是多个知名会议的程序委员会成员，如DAC、DATE、ASP-DAC、FPGA、FPL和FPT等。他曾获得两项IEEE/ACM最佳论文奖。他是IEEE高级会员（Senior Member）。